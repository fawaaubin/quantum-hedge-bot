// Package risk implémente la gestion du risque : dimensionnement des
// positions, validation des entrées, niveaux de sortie (stop loss, take
// profit, stop suiveur) et circuit breaker sur pertes récentes.
//
// Le manager est autonome et sans dépendance exchange : le capital de
// référence vient de la configuration et évolue avec les PnL enregistrés.
// Le circuit breaker se déclenche quand la perte nette des N derniers
// trades dépasse le seuil, et se lève après un cooldown configurable
// (sans cooldown, la suspension serait définitive : plus aucun trade ne
// ferait évoluer la fenêtre).
package risk

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Erreurs typées de validation d'entrée.
var (
	ErrBreakerActive   = errors.New("circuit breaker actif: trading suspendu")
	ErrMaxPositions    = errors.New("nombre maximal de positions ouvertes atteint")
	ErrMaxAbsoluteRisk = errors.New("risque absolu maximal dépassé")
	ErrInvalidSignal   = errors.New("signal d'entrée invalide")
)

// EventSink reçoit les événements de risque (branché sur le store en
// Phase 6). Ne doit jamais bloquer.
type EventSink func(types.RiskEvent)

// Manager implémente types.RiskManager.
type Manager struct {
	cfg  config.RiskConfig
	log  *slog.Logger
	now  func() time.Time // injectable pour les tests
	sink EventSink

	mu            sync.Mutex
	capital       float64
	openPositions int
	committedRisk float64   // risque engagé par les positions ouvertes
	window        []float64 // PnL des derniers trades (taille BreakerLookback)
	breakerUntil  time.Time // fin de suspension; zéro si inactif
}

var _ types.RiskManager = (*Manager)(nil)

// New construit le risk manager avec le capital initial de la config.
func New(cfg config.RiskConfig, log *slog.Logger) *Manager {
	return &Manager{
		cfg:     cfg,
		log:     log.With("module", "risk"),
		now:     time.Now,
		capital: cfg.InitialCapital,
	}
}

// SetEventSink branche la persistance des événements de risque.
func (m *Manager) SetEventSink(sink EventSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sink = sink
}

// Capital retourne le capital de référence courant.
func (m *Manager) Capital() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.capital
}

// PositionSize calcule la taille de position pour une entrée LONG :
// (capital × risque par trade) / (entry − stopLoss), plafonnée par le
// capital disponible (aucun levier).
func (m *Manager) PositionSize(ctx context.Context, entry, stopLoss, capital float64) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if capital <= 0 {
		return 0, fmt.Errorf("capital invalide: %v", capital)
	}
	if entry <= 0 || stopLoss <= 0 {
		return 0, fmt.Errorf("prix invalides: entry=%v stopLoss=%v", entry, stopLoss)
	}
	riskPerUnit := entry - stopLoss
	if riskPerUnit <= 0 {
		return 0, fmt.Errorf("stop loss %v >= prix d'entrée %v (LONG uniquement)", stopLoss, entry)
	}

	qty := capital * m.cfg.MaxRiskPerTrade / riskPerUnit

	// Sans levier, le notionnel ne peut pas dépasser le capital.
	if maxQty := capital / entry; qty > maxQty {
		qty = maxQty
	}
	m.log.Debug("taille de position calculée",
		"entry", entry, "stop_loss", stopLoss, "capital", capital, "qty", qty)
	return qty, nil
}

// ValidateEntry vérifie qu'un signal d'entrée respecte toutes les
// contraintes : circuit breaker, nombre de positions, risque absolu.
func (m *Manager) ValidateEntry(ctx context.Context, sig types.Signal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if sig.Action != types.ActionEnterLong || sig.Price <= 0 {
		return fmt.Errorf("%w: action=%s price=%v", ErrInvalidSignal, sig.Action, sig.Price)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.breakerActiveLocked() {
		m.emitLocked(types.RiskEventRejectedEntry, "entrée refusée: circuit breaker actif")
		return ErrBreakerActive
	}
	if m.openPositions >= m.cfg.MaxOpenPositions {
		m.emitLocked(types.RiskEventRejectedEntry,
			fmt.Sprintf("entrée refusée: %d position(s) déjà ouverte(s)", m.openPositions))
		return ErrMaxPositions
	}
	newRisk := m.capital * m.cfg.MaxRiskPerTrade
	if m.committedRisk+newRisk > m.capital*m.cfg.MaxAbsoluteRisk+1e-9 {
		m.emitLocked(types.RiskEventMaxRisk,
			fmt.Sprintf("entrée refusée: risque engagé %.2f + nouveau %.2f > max %.2f",
				m.committedRisk, newRisk, m.capital*m.cfg.MaxAbsoluteRisk))
		return ErrMaxAbsoluteRisk
	}
	return nil
}

// PositionOpened enregistre l'ouverture d'une position et le risque
// qu'elle engage. Appelé par le moteur après exécution de l'entrée.
func (m *Manager) PositionOpened(riskAmount float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.openPositions++
	m.committedRisk += riskAmount
	m.log.Info("position ouverte enregistrée",
		"open_positions", m.openPositions, "committed_risk", m.committedRisk)
}

// ExitLevels calcule les niveaux de sortie initiaux d'une position :
// stop loss (−1 %), take profit (+2 %) ; le stop suiveur est inactif
// tant que le seuil d'activation n'est pas atteint (voir UpdateTrailing).
func (m *Manager) ExitLevels(ctx context.Context, pos types.Position) (types.ExitLevels, error) {
	if err := ctx.Err(); err != nil {
		return types.ExitLevels{}, err
	}
	if pos.AvgEntry <= 0 {
		return types.ExitLevels{}, fmt.Errorf("prix moyen d'entrée invalide: %v", pos.AvgEntry)
	}
	return types.ExitLevels{
		StopLoss:     pos.AvgEntry * (1 - m.cfg.StopLossPct),
		TakeProfit:   pos.AvgEntry * (1 + m.cfg.TakeProfitPct),
		TrailingStop: pos.TrailingStop, // conservé tel quel (ratchet)
	}, nil
}

// ExitFor est le calculateur branché sur l'order manager : recalcul de
// SL/TP à chaque changement du prix moyen d'entrée.
func (m *Manager) ExitFor(avgEntry float64) (stopLoss, takeProfit float64) {
	return avgEntry * (1 - m.cfg.StopLossPct), avgEntry * (1 + m.cfg.TakeProfitPct)
}

// UpdateTrailing fait progresser le stop suiveur d'une position LONG :
//
//   - inactif tant que le prix n'a pas atteint entry × (1 + activation) ;
//   - une fois actif, stop = prix × (1 − pas), à cliquet (jamais abaissé) ;
//   - triggered devient vrai quand le prix repasse sous le stop.
//
// Retourne le nouveau stop (0 si inactif) et l'état de déclenchement.
func (m *Manager) UpdateTrailing(pos types.Position, price float64) (stop float64, triggered bool) {
	if pos.AvgEntry <= 0 || price <= 0 {
		return pos.TrailingStop, false
	}
	stop = pos.TrailingStop

	if stop == 0 {
		// Activation à partir de +TrailingActivePct de profit.
		if price < pos.AvgEntry*(1+m.cfg.TrailingActivePct) {
			return 0, false
		}
	}
	if candidate := price * (1 - m.cfg.TrailingStepPct); candidate > stop {
		stop = candidate
	}
	return stop, price <= stop
}

// RecordTradeResult enregistre un trade clôturé : mise à jour du capital,
// libération du risque engagé, alimentation de la fenêtre du circuit
// breaker et déclenchement éventuel de la suspension.
func (m *Manager) RecordTradeResult(ctx context.Context, res types.TradeResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.capital += res.PnL
	if m.openPositions > 0 {
		m.openPositions--
	}
	m.committedRisk = 0 // avec MaxOpenPositions=1, tout le risque est libéré

	m.window = append(m.window, res.PnL)
	if len(m.window) > m.cfg.BreakerLookback {
		m.window = m.window[len(m.window)-m.cfg.BreakerLookback:]
	}

	var net float64
	for _, pnl := range m.window {
		net += pnl
	}
	m.log.Info("trade enregistré",
		"pnl", res.PnL, "capital", m.capital,
		"window_net", net, "window_size", len(m.window), "reason", res.ReasonTag)

	if !m.breakerActiveLocked() && net < 0 && -net >= m.capital*m.cfg.BreakerLossPct {
		m.breakerUntil = m.now().Add(m.cfg.BreakerCooldown)
		detail := fmt.Sprintf(
			"circuit breaker déclenché: perte nette %.2f sur %d trades (seuil %.2f), suspension jusqu'à %s",
			-net, len(m.window), m.capital*m.cfg.BreakerLossPct,
			m.breakerUntil.UTC().Format(time.RFC3339))
		m.log.Warn(detail)
		m.emitLocked(types.RiskEventCircuitBreaker, detail)
	}
	return nil
}

// CircuitBreakerActive indique si le trading est actuellement suspendu.
func (m *Manager) CircuitBreakerActive(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.breakerActiveLocked(), nil
}

// ResetBreaker lève manuellement la suspension et purge la fenêtre.
func (m *Manager) ResetBreaker() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.breakerUntil = time.Time{}
	m.window = nil
	m.log.Warn("circuit breaker réinitialisé manuellement")
}

// breakerActiveLocked évalue l'état de la suspension. m.mu détenu.
func (m *Manager) breakerActiveLocked() bool {
	if m.breakerUntil.IsZero() {
		return false
	}
	if m.now().Before(m.breakerUntil) {
		return true
	}
	// Cooldown écoulé : levée de la suspension et purge de la fenêtre
	// pour ne pas redéclencher immédiatement sur les mêmes pertes.
	m.breakerUntil = time.Time{}
	m.window = nil
	m.log.Info("circuit breaker levé (fin du cooldown)")
	return false
}

// emitLocked journalise et propage un événement de risque. m.mu détenu.
func (m *Manager) emitLocked(kind types.RiskEventKind, detail string) {
	ev := types.RiskEvent{Kind: kind, Detail: detail, Timestamp: m.now().UTC()}
	m.log.Warn("événement de risque", "kind", string(kind), "detail", detail)
	if m.sink != nil {
		go m.sink(ev) // jamais bloquant pour le chemin de décision
	}
}
