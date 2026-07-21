// Package ordermanager exécute, annule, remplace et suit les ordres.
//
// Phase 1 : squelette sans aucune logique d'exécution réelle.
package ordermanager

import (
	"context"
	"log/slog"

	"github.com/fawaaubin/quantum-hedge-bot/internal/binance"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Manager implémente types.OrderExecutor.
type Manager struct {
	client *binance.Client
	log    *slog.Logger
}

var _ types.OrderExecutor = (*Manager)(nil)

// New construit l'order manager au-dessus du client Binance.
func New(client *binance.Client, log *slog.Logger) *Manager {
	return &Manager{client: client, log: log.With("module", "ordermanager")}
}

// PlaceOrder sera implémenté en Phase 3 (LIMIT GTC, signature, filtres).
func (m *Manager) PlaceOrder(ctx context.Context, req types.OrderRequest) (types.Order, error) {
	return types.Order{}, types.ErrNotImplemented
}

// CancelOrder sera implémenté en Phase 3.
func (m *Manager) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	return types.ErrNotImplemented
}

// ReplaceOrder sera implémenté en Phase 3 (vérification d'état préalable).
func (m *Manager) ReplaceOrder(ctx context.Context, symbol string, orderID int64, req types.OrderRequest) (types.Order, error) {
	return types.Order{}, types.ErrNotImplemented
}

// OpenOrders sera implémenté en Phase 3.
func (m *Manager) OpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	return nil, types.ErrNotImplemented
}

// Reconcile sera implémenté en Phase 3 (réconciliation au démarrage,
// prévention des doubles ordres).
func (m *Manager) Reconcile(ctx context.Context) error {
	return types.ErrNotImplemented
}
