// Package ordermanager exécute, annule, remplace et suit les ordres.
//
// Règles d'exécution (cahier des charges) :
//   - les ordres d'entrée sont obligatoirement LIMIT GTC ;
//   - un ordre non exécuté dont le prix dérive au-delà du seuil est
//     annulé puis replacé immédiatement (après vérification d'état) ;
//   - les remplissages partiels alimentent la position (prix moyen exact
//     via cummulativeQuoteQty) ; règle du reliquat : à chaque
//     remplacement, le reliquat est annulé et replacé intégralement au
//     nouveau prix ;
//   - idempotence : ClientOrderID préfixé, réconciliation openOrders +
//     account au démarrage, vérification d'existence avant toute création.
package ordermanager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"

	"github.com/fawaaubin/quantum-hedge-bot/internal/binance"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// clientIDPrefix identifie les ordres créés par ce bot : la réconciliation
// n'adopte que les ordres portant ce préfixe.
const clientIDPrefix = "qhb-"

// ErrOrderExists est retourné quand un ordre d'entrée actif existe déjà
// (prévention des doubles ordres).
var ErrOrderExists = errors.New("un ordre d'entrée actif existe déjà")

// ErrRemainderDust est retourné quand le reliquat d'un remplacement est
// trop petit pour être replacé (sous minQty ou minNotional).
var ErrRemainderDust = errors.New("reliquat trop petit pour être replacé")

// ErrNotReconciled est retourné si une opération d'ordre est tentée
// avant la réconciliation initiale.
var ErrNotReconciled = errors.New("réconciliation initiale non effectuée")

// restClient est la surface du client Binance consommée par le manager.
// Interface locale pour la testabilité.
type restClient interface {
	PlaceOrder(ctx context.Context, req types.OrderRequest, f types.SymbolFilters) (types.Order, error)
	CancelOrder(ctx context.Context, symbol string, orderID int64) error
	QueryOrder(ctx context.Context, symbol string, orderID int64) (types.Order, error)
	OpenOrders(ctx context.Context, symbol string) ([]types.Order, error)
	Account(ctx context.Context) ([]types.Balance, error)
	ExchangeFilters(ctx context.Context, symbol string) (types.SymbolFilters, error)
}

// ExitCalculator recalcule les niveaux de sortie après tout changement
// du prix moyen d'entrée. Branché par le risk manager en Phase 4.
type ExitCalculator func(avgEntry float64) (stopLoss, takeProfit float64)

// Manager implémente types.OrderExecutor pour un symbole unique.
type Manager struct {
	client   restClient
	symbol   string
	replPct  float64 // seuil de dérive avant remplacement (fraction)
	log      *slog.Logger
	exitCalc ExitCalculator

	mu         sync.Mutex
	reconciled bool
	filters    types.SymbolFilters
	entryOrder *types.Order // ordre d'entrée actif, nil sinon
	// Accumulateurs de position : quantité exécutée et coût quote cumulé
	// (sources : executedQty et cummulativeQuoteQty des ordres).
	posQty  float64
	posCost float64
}

var _ types.OrderExecutor = (*Manager)(nil)

// New construit l'order manager.
func New(client restClient, symbol string, replaceThresholdPct float64, log *slog.Logger) *Manager {
	return &Manager{
		client:  client,
		symbol:  symbol,
		replPct: replaceThresholdPct,
		log:     log.With("module", "ordermanager", "symbol", symbol),
	}
}

// SetExitCalculator branche le recalcul SL/TP (risk manager, Phase 4).
func (m *Manager) SetExitCalculator(calc ExitCalculator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exitCalc = calc
}

// Reconcile charge les filtres du symbole puis réconcilie l'état local
// avec openOrders et account : les ordres du bot (préfixe ClientOrderID)
// sont adoptés, ce qui empêche tout double ordre après redémarrage.
func (m *Manager) Reconcile(ctx context.Context) error {
	filters, err := m.client.ExchangeFilters(ctx, m.symbol)
	if err != nil {
		return fmt.Errorf("chargement des filtres: %w", err)
	}

	open, err := m.client.OpenOrders(ctx, m.symbol)
	if err != nil {
		return fmt.Errorf("openOrders: %w", err)
	}

	balances, err := m.client.Account(ctx)
	if err != nil {
		return fmt.Errorf("account: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.filters = filters
	m.entryOrder = nil
	for i := range open {
		o := open[i]
		if len(o.ClientOrderID) >= len(clientIDPrefix) && o.ClientOrderID[:len(clientIDPrefix)] == clientIDPrefix {
			if m.entryOrder != nil {
				m.log.Warn("plusieurs ordres du bot ouverts, adoption du plus récent",
					"order_id", o.OrderID)
				if o.CreatedAt.Before(m.entryOrder.CreatedAt) {
					continue
				}
			}
			m.applyExecutionLocked(o)
			m.entryOrder = &o
		} else {
			m.log.Warn("ordre ouvert étranger au bot, ignoré",
				"order_id", o.OrderID, "client_order_id", o.ClientOrderID)
		}
	}
	m.reconciled = true

	m.log.Info("réconciliation terminée",
		"open_orders", len(open),
		"adopted", m.entryOrder != nil,
		"balances", len(balances),
		"position_qty", m.posQty,
	)
	return nil
}

// PlaceOrder place un ordre d'entrée LIMIT GTC après validation des
// filtres. Refuse toute création si un ordre d'entrée actif existe déjà
// (localement ou côté exchange).
func (m *Manager) PlaceOrder(ctx context.Context, req types.OrderRequest) (types.Order, error) {
	m.mu.Lock()
	if !m.reconciled {
		m.mu.Unlock()
		return types.Order{}, ErrNotReconciled
	}
	if req.Type != types.OrderTypeLimit {
		m.mu.Unlock()
		return types.Order{}, fmt.Errorf("les ordres d'entrée doivent être LIMIT (reçu %s)", req.Type)
	}
	if req.TimeInForce == "" {
		req.TimeInForce = types.TimeInForceGTC
	}
	if req.TimeInForce != types.TimeInForceGTC {
		m.mu.Unlock()
		return types.Order{}, fmt.Errorf("les ordres d'entrée doivent être GTC (reçu %s)", req.TimeInForce)
	}
	if m.entryOrder != nil && m.entryOrder.IsOpen() {
		m.mu.Unlock()
		return types.Order{}, ErrOrderExists
	}
	filters := m.filters
	m.mu.Unlock()

	// Vérification d'existence côté exchange avant toute création :
	// un ordre du bot encore ouvert interdit un nouveau placement.
	open, err := m.client.OpenOrders(ctx, m.symbol)
	if err != nil {
		return types.Order{}, fmt.Errorf("vérification openOrders: %w", err)
	}
	for _, o := range open {
		if len(o.ClientOrderID) >= len(clientIDPrefix) && o.ClientOrderID[:len(clientIDPrefix)] == clientIDPrefix {
			return types.Order{}, fmt.Errorf("%w (order_id=%d)", ErrOrderExists, o.OrderID)
		}
	}

	req.Symbol = m.symbol
	if req.ClientOrderID == "" {
		req.ClientOrderID = newClientOrderID()
	}
	req, err = binance.ValidateAndQuantize(req, filters)
	if err != nil {
		return types.Order{}, fmt.Errorf("validation des filtres: %w", err)
	}

	order, err := m.client.PlaceOrder(ctx, req, filters)
	if err != nil {
		return types.Order{}, err
	}

	m.mu.Lock()
	m.applyExecutionLocked(order)
	m.entryOrder = &order
	m.mu.Unlock()

	m.log.Info("ordre placé", "order_id", order.OrderID,
		"price", order.Price, "qty", order.OrigQuantity, "status", string(order.Status))
	return order, nil
}

// CancelOrder annule un ordre. Un ordre déjà disparu côté exchange
// (exécuté ou annulé) n'est pas une erreur : l'état est rafraîchi.
func (m *Manager) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	if err := m.client.CancelOrder(ctx, symbol, orderID); err != nil {
		if binance.IsUnknownOrder(err) {
			m.log.Warn("annulation d'un ordre déjà clos", "order_id", orderID)
			return m.refreshOrder(ctx, orderID)
		}
		return err
	}
	m.mu.Lock()
	if m.entryOrder != nil && m.entryOrder.OrderID == orderID {
		m.entryOrder.Status = types.OrderStatusCanceled
	}
	m.mu.Unlock()
	return nil
}

// ReplaceOrder remplace un ordre au nouveau prix demandé, après
// vérification de son état réel :
//
//   - FILLED : aucun remplacement, la position est mise à jour ;
//   - NEW / PARTIALLY_FILLED : la quantité déjà exécutée alimente la
//     position, le reliquat est annulé puis replacé intégralement au
//     nouveau prix (règle du reliquat) ;
//   - reliquat sous minQty/minNotional : ErrRemainderDust.
func (m *Manager) ReplaceOrder(ctx context.Context, symbol string, orderID int64, req types.OrderRequest) (types.Order, error) {
	m.mu.Lock()
	if !m.reconciled {
		m.mu.Unlock()
		return types.Order{}, ErrNotReconciled
	}
	filters := m.filters
	m.mu.Unlock()

	current, err := m.client.QueryOrder(ctx, symbol, orderID)
	if err != nil {
		return types.Order{}, fmt.Errorf("vérification de l'ordre %d: %w", orderID, err)
	}

	m.mu.Lock()
	m.applyExecutionLocked(current)
	m.mu.Unlock()

	if current.Status == types.OrderStatusFilled {
		m.log.Info("ordre déjà exécuté, remplacement inutile", "order_id", orderID)
		m.mu.Lock()
		m.entryOrder = &current
		m.mu.Unlock()
		return current, nil
	}
	if !current.IsOpen() {
		return types.Order{}, fmt.Errorf("ordre %d non remplaçable (statut %s)", orderID, current.Status)
	}

	if err := m.CancelOrder(ctx, symbol, orderID); err != nil {
		return types.Order{}, fmt.Errorf("annulation avant remplacement: %w", err)
	}

	remaining := current.OrigQuantity - current.ExecutedQuantity
	req.Symbol = symbol
	req.Type = types.OrderTypeLimit
	req.TimeInForce = types.TimeInForceGTC
	req.Quantity = remaining
	req.ClientOrderID = newClientOrderID()

	req, err = binance.ValidateAndQuantize(req, filters)
	if err != nil {
		m.mu.Lock()
		m.entryOrder = nil
		m.mu.Unlock()
		return types.Order{}, fmt.Errorf("%w: %v", ErrRemainderDust, err)
	}

	order, err := m.client.PlaceOrder(ctx, req, filters)
	if err != nil {
		return types.Order{}, fmt.Errorf("replacement: %w", err)
	}

	m.mu.Lock()
	m.applyExecutionLocked(order)
	m.entryOrder = &order
	m.mu.Unlock()

	m.log.Info("ordre remplacé", "old_order_id", orderID, "new_order_id", order.OrderID,
		"price", order.Price, "qty", order.OrigQuantity)
	return order, nil
}

// OpenOrders liste les ordres ouverts du symbole côté exchange.
func (m *Manager) OpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	return m.client.OpenOrders(ctx, symbol)
}

// ShouldReplace indique si le meilleur prix s'est éloigné du prix de
// l'ordre au-delà du seuil configuré (0,01 % par défaut).
func (m *Manager) ShouldReplace(orderPrice, bestPrice float64) bool {
	if orderPrice <= 0 {
		return false
	}
	return math.Abs(bestPrice-orderPrice)/orderPrice > m.replPct
}

// Position retourne l'état courant de la position : quantité exécutée,
// prix moyen d'entrée exact et niveaux de sortie recalculés.
func (m *Manager) Position() types.Position {
	m.mu.Lock()
	defer m.mu.Unlock()

	pos := types.Position{Symbol: m.symbol, Side: types.SideBuy, Quantity: m.posQty}
	if m.posQty > 0 {
		pos.AvgEntry = m.posCost / m.posQty
		if m.exitCalc != nil {
			pos.StopLoss, pos.TakeProfit = m.exitCalc(pos.AvgEntry)
		}
	}
	return pos
}

// EntryOrder retourne une copie de l'ordre d'entrée actif, s'il existe.
func (m *Manager) EntryOrder() (types.Order, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entryOrder == nil {
		return types.Order{}, false
	}
	return *m.entryOrder, true
}

// refreshOrder resynchronise l'état local d'un ordre depuis l'exchange.
func (m *Manager) refreshOrder(ctx context.Context, orderID int64) error {
	order, err := m.client.QueryOrder(ctx, m.symbol, orderID)
	if err != nil {
		return fmt.Errorf("rafraîchissement de l'ordre %d: %w", orderID, err)
	}
	m.mu.Lock()
	m.applyExecutionLocked(order)
	m.entryOrder = &order
	m.mu.Unlock()
	return nil
}

// applyExecutionLocked intègre la progression d'exécution d'un ordre
// dans la position : seule la variation depuis le dernier état connu est
// ajoutée (delta d'executedQty et de cummulativeQuoteQty), ce qui rend
// l'opération idempotente. m.mu doit être détenu.
func (m *Manager) applyExecutionLocked(order types.Order) {
	var prevQty, prevCost float64
	if m.entryOrder != nil && m.entryOrder.OrderID == order.OrderID {
		prevQty = m.entryOrder.ExecutedQuantity
		prevCost = m.entryOrder.CumQuote
	}
	dQty := order.ExecutedQuantity - prevQty
	dCost := order.CumQuote - prevCost
	if dQty > 0 {
		m.posQty += dQty
		m.posCost += dCost
		m.log.Info("exécution intégrée à la position",
			"order_id", order.OrderID, "delta_qty", dQty,
			"position_qty", m.posQty, "avg_entry", m.posCost/m.posQty)
	}
}

// newClientOrderID génère un identifiant client idempotent et traçable.
func newClientOrderID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read sur crypto/rand n'échoue jamais en pratique.
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return clientIDPrefix + hex.EncodeToString(b[:])
}
