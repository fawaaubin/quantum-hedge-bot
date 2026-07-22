//go:build integration

// Test d'intégration contre le Binance Spot Testnet réel.
//
// Prérequis : BINANCE_API_KEY et BINANCE_API_SECRET (clés testnet) dans
// l'environnement. Lancement :
//
//	go test -tags integration -run TestIntegration ./internal/ordermanager/
//
// Le test place un ordre LIMIT GTC 20 % sous le marché (jamais exécuté),
// vérifie sa présence dans openOrders, puis l'annule.
package ordermanager

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/binance"
	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

func TestIntegrationPlaceAndCancel(t *testing.T) {
	if os.Getenv("BINANCE_API_KEY") == "" || os.Getenv("BINANCE_API_SECRET") == "" {
		t.Skip("BINANCE_API_KEY / BINANCE_API_SECRET absents")
	}

	cfg := config.BinanceConfig{
		RESTBaseURL:        "https://testnet.binance.vision",
		WSBaseURL:          "wss://stream.testnet.binance.vision",
		RecvWindow:         5 * time.Second,
		Testnet:            true,
		RateLimitCapacity:  5,
		RateLimitRefillPer: 2,
		APIKey:             os.Getenv("BINANCE_API_KEY"),
		APISecret:          os.Getenv("BINANCE_API_SECRET"),
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	client := binance.NewClient(cfg, log)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m := New(client, "BTCUSDT", 0.0001, log)
	if err := m.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Prix de référence : snapshot du carnet, ordre à -20 % (non exécutable).
	ob, err := client.DepthSnapshot(ctx, "BTCUSDT", 5)
	if err != nil {
		t.Fatalf("DepthSnapshot: %v", err)
	}
	if len(ob.Bids) == 0 {
		t.Fatal("carnet vide")
	}
	price := ob.Bids[0].Price * 0.80
	qty := 6.0 / price // ~6 USDT de notionnel (minNotional testnet: 5)

	order, err := m.PlaceOrder(ctx, types.OrderRequest{
		Side:     types.SideBuy,
		Type:     types.OrderTypeLimit,
		Price:    price,
		Quantity: qty,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	t.Logf("ordre placé: id=%d price=%v qty=%v", order.OrderID, order.Price, order.OrigQuantity)

	// L'ordre doit apparaître dans openOrders.
	open, err := m.OpenOrders(ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("OpenOrders: %v", err)
	}
	found := false
	for _, o := range open {
		if o.OrderID == order.OrderID {
			found = true
		}
	}
	if !found {
		t.Errorf("ordre %d absent d'openOrders", order.OrderID)
	}

	// Un second placement doit être refusé (anti double ordre).
	if _, err := m.PlaceOrder(ctx, types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: price, Quantity: qty,
	}); err == nil {
		t.Error("le double ordre aurait dû être refusé")
	}

	// Nettoyage : annulation.
	if err := m.CancelOrder(ctx, "BTCUSDT", order.OrderID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	t.Logf("ordre %d annulé", order.OrderID)
}
