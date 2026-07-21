package binance

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
)

func newTestClient(baseURL string) *Client {
	return NewClient(config.BinanceConfig{
		RESTBaseURL: baseURL,
		RecvWindow:  5 * time.Second,
		Testnet:     true,
	}, slog.New(slog.DiscardHandler))
}

func TestDepthSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/depth" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "BTCUSDT" {
			t.Errorf("symbol = %s", got)
		}
		if got := r.URL.Query().Get("limit"); got != "500" {
			t.Errorf("limit = %s", got)
		}
		w.Write([]byte(`{"lastUpdateId":42,"bids":[["60000.00","1.5"]],"asks":[["60001.00","2.0"]]}`))
	}))
	defer srv.Close()

	ob, err := newTestClient(srv.URL).DepthSnapshot(context.Background(), "BTCUSDT", 500)
	if err != nil {
		t.Fatalf("DepthSnapshot: %v", err)
	}
	if ob.LastUpdateID != 42 {
		t.Errorf("lastUpdateId = %d, attendu 42", ob.LastUpdateID)
	}
	if len(ob.Bids) != 1 || ob.Bids[0].Price != 60000 || ob.Bids[0].Quantity != 1.5 {
		t.Errorf("bids incorrects: %+v", ob.Bids)
	}
	if len(ob.Asks) != 1 || ob.Asks[0].Price != 60001 {
		t.Errorf("asks incorrects: %+v", ob.Asks)
	}
}

func TestDepthSnapshotHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte(`{"code":-1121,"msg":"Invalid symbol."}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).DepthSnapshot(context.Background(), "NOPE", 100); err == nil {
		t.Fatal("un statut HTTP non 200 doit retourner une erreur")
	}
}

func TestDepthSnapshotContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := newTestClient(srv.URL).DepthSnapshot(ctx, "BTCUSDT", 100); err == nil {
		t.Fatal("l'annulation du contexte doit interrompre l'appel")
	}
}
