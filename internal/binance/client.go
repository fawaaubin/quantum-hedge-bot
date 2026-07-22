// Package binance contient le client REST/WebSocket Binance et la
// validation des filtres. Aucune logique métier ne doit vivre ici.
package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Client est le client bas niveau de l'API Binance Spot Testnet.
// Le http.Client est configuré en keep-alive avec un pool de connexions ;
// les requêtes signées passent par un token bucket configurable.
type Client struct {
	cfg     config.BinanceConfig
	http    *http.Client
	log     *slog.Logger
	limiter *tokenBucket
	now     func() time.Time // injectable pour les tests
}

// NewClient construit le client REST avec un transport keep-alive.
func NewClient(cfg config.BinanceConfig, log *slog.Logger) *Client {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
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
		log:     log.With("module", "binance"),
		limiter: newTokenBucket(cfg.RateLimitCapacity, cfg.RateLimitRefillPer),
		now:     time.Now,
	}
}

// depthResponse est la réponse brute de GET /api/v3/depth.
type depthResponse struct {
	LastUpdateID int64       `json:"lastUpdateId"`
	Bids         [][2]string `json:"bids"`
	Asks         [][2]string `json:"asks"`
}

// DepthSnapshot récupère le snapshot REST du carnet d'ordres.
// L'appel est borné par le contexte fourni (timeout, annulation).
func (c *Client) DepthSnapshot(ctx context.Context, symbol string, limit int) (types.OrderBook, error) {
	endpoint := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d",
		c.cfg.RESTBaseURL, url.QueryEscape(symbol), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return types.OrderBook{}, fmt.Errorf("requête depth: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return types.OrderBook{}, fmt.Errorf("appel depth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return types.OrderBook{}, fmt.Errorf("depth HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw depthResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return types.OrderBook{}, fmt.Errorf("décodage depth: %w", err)
	}

	bids, err := parseLevels(raw.Bids)
	if err != nil {
		return types.OrderBook{}, fmt.Errorf("bids snapshot: %w", err)
	}
	asks, err := parseLevels(raw.Asks)
	if err != nil {
		return types.OrderBook{}, fmt.Errorf("asks snapshot: %w", err)
	}

	return types.OrderBook{
		Symbol:       symbol,
		LastUpdateID: raw.LastUpdateID,
		Bids:         bids,
		Asks:         asks,
		UpdatedAt:    time.Now().UTC(),
	}, nil
}

// parseLevels convertit les niveaux [prix, quantité] de l'API REST.
func parseLevels(in [][2]string) ([]types.PriceLevel, error) {
	out := make([]types.PriceLevel, 0, len(in))
	for _, pair := range in {
		price, err := strconv.ParseFloat(pair[0], 64)
		if err != nil {
			return nil, fmt.Errorf("prix %q: %w", pair[0], err)
		}
		qty, err := strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return nil, fmt.Errorf("quantité %q: %w", pair[1], err)
		}
		out = append(out, types.PriceLevel{Price: price, Quantity: qty})
	}
	return out, nil
}
