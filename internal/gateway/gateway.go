// Package gateway gère les WebSocket publics (et privés en phase
// ultérieure) Binance et publie les événements de marché normalisés.
//
// Phase 1 : squelette sans aucune logique WebSocket réelle.
package gateway

import (
	"context"
	"log/slog"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Gateway implémente types.MarketDataSource.
type Gateway struct {
	cfg    config.GatewayConfig
	symbol string
	log    *slog.Logger
	events chan types.MarketEvent
}

var _ types.MarketDataSource = (*Gateway)(nil)

// New construit une gateway pour un symbole unique.
func New(cfg config.GatewayConfig, symbol string, log *slog.Logger) *Gateway {
	return &Gateway{
		cfg:    cfg,
		symbol: symbol,
		log:    log.With("module", "gateway", "symbol", symbol),
		events: make(chan types.MarketEvent, cfg.EventBufferSize),
	}
}

// Start sera implémenté en Phase 2 (flux trade + depth, snapshot REST,
// resynchronisation, reconnexion avec backoff).
func (g *Gateway) Start(ctx context.Context) error {
	return types.ErrNotImplemented
}

// Events expose le channel bufferisé d'événements de marché.
func (g *Gateway) Events() <-chan types.MarketEvent {
	return g.events
}

// OrderBook sera implémenté en Phase 2 (carnet local reconstruit).
func (g *Gateway) OrderBook(ctx context.Context, symbol string) (types.OrderBook, error) {
	return types.OrderBook{}, types.ErrNotImplemented
}

// Stop sera implémenté en Phase 2 (fermeture propre des connexions).
func (g *Gateway) Stop(ctx context.Context) error {
	return types.ErrNotImplemented
}
