// Package binance contient le client REST/WebSocket Binance et la
// validation des filtres. Aucune logique métier ne doit vivre ici.
//
// Phase 1 : squelette sans aucun appel réseau réel.
package binance

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Client est le client bas niveau de l'API Binance Spot Testnet.
// Le http.Client est configuré en keep-alive avec un pool de connexions.
type Client struct {
	cfg  config.BinanceConfig
	http *http.Client
	log  *slog.Logger
}

// NewClient construit le client REST avec un transport keep-alive.
func NewClient(cfg config.BinanceConfig, log *slog.Logger) *Client {
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		log: log.With("module", "binance"),
	}
}

// DepthSnapshot récupérera le snapshot REST du carnet (Phase 2).
func (c *Client) DepthSnapshot(ctx context.Context, symbol string, limit int) (types.OrderBook, error) {
	return types.OrderBook{}, types.ErrNotImplemented
}

// ExchangeFilters récupérera et validera les filtres du symbole (Phase 3).
func (c *Client) ExchangeFilters(ctx context.Context, symbol string) (types.SymbolFilters, error) {
	return types.SymbolFilters{}, types.ErrNotImplemented
}
