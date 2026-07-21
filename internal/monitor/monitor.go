// Package monitor expose /health et /metrics, envoie les alertes
// Telegram et centralise l'observabilité.
//
// Phase 1 : squelette sans serveur HTTP ni alertes réelles.
package monitor

import (
	"context"
	"log/slog"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Monitor implémente types.Notifier et portera les endpoints HTTP (Phase 6).
type Monitor struct {
	cfg config.MonitorConfig
	log *slog.Logger
}

var _ types.Notifier = (*Monitor)(nil)

// New construit le monitor.
func New(cfg config.MonitorConfig, log *slog.Logger) *Monitor {
	return &Monitor{cfg: cfg, log: log.With("module", "monitor")}
}

// Notify sera implémenté en Phase 6 (Telegram non bloquant). En Phase 1,
// l'alerte est simplement journalisée pour ne jamais bloquer le bot.
func (m *Monitor) Notify(ctx context.Context, level types.AlertLevel, message string) error {
	m.log.Info("alerte", "level", string(level), "message", message)
	return nil
}

// Serve démarrera le serveur /health et /metrics en Phase 6.
func (m *Monitor) Serve(ctx context.Context) error {
	return types.ErrNotImplemented
}
