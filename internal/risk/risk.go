// Package risk implémente la gestion du risque : sizing, niveaux de
// sortie et circuit breaker.
//
// Phase 1 : squelette sans aucune logique de risque réelle.
package risk

import (
	"context"
	"log/slog"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Manager implémente types.RiskManager.
type Manager struct {
	cfg config.RiskConfig
	log *slog.Logger
}

var _ types.RiskManager = (*Manager)(nil)

// New construit le risk manager.
func New(cfg config.RiskConfig, log *slog.Logger) *Manager {
	return &Manager{cfg: cfg, log: log.With("module", "risk")}
}

// PositionSize sera implémenté en Phase 4 (1 % du capital par trade).
func (m *Manager) PositionSize(ctx context.Context, entry, stopLoss, capital float64) (float64, error) {
	return 0, types.ErrNotImplemented
}

// ValidateEntry sera implémenté en Phase 4.
func (m *Manager) ValidateEntry(ctx context.Context, sig types.Signal) error {
	return types.ErrNotImplemented
}

// ExitLevels sera implémenté en Phase 4 (SL 1 %, TP 2 %, trailing).
func (m *Manager) ExitLevels(ctx context.Context, pos types.Position) (types.ExitLevels, error) {
	return types.ExitLevels{}, types.ErrNotImplemented
}

// RecordTradeResult sera implémenté en Phase 4 (fenêtre des 10 derniers trades).
func (m *Manager) RecordTradeResult(ctx context.Context, res types.TradeResult) error {
	return types.ErrNotImplemented
}

// CircuitBreakerActive sera implémenté en Phase 4 (pertes > 5 % du capital).
func (m *Manager) CircuitBreakerActive(ctx context.Context) (bool, error) {
	return false, types.ErrNotImplemented
}
