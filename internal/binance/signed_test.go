package binance

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
)

// TestSignOfficialVector vérifie la signature HMAC-SHA256 avec le
// vecteur d'exemple de la documentation officielle Binance.
func TestSignOfficialVector(t *testing.T) {
	secret := "NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j"
	payload := "symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000&timestamp=1499827319559"
	want := "c8db56825ae71d6d79447849e617115f4a920fa2acdcab2b053c4b2838bd6b71"

	if got := sign(payload, secret); got != want {
		t.Errorf("signature = %s, attendu %s", got, want)
	}
}

func signedTestClient(baseURL string) *Client {
	c := NewClient(config.BinanceConfig{
		RESTBaseURL:        baseURL,
		RecvWindow:         5 * time.Second,
		Testnet:            true,
		RateLimitCapacity:  100,
		RateLimitRefillPer: 100,
		APIKey:             "test-api-key",
		APISecret:          "test-secret",
	}, slog.New(slog.DiscardHandler))
	c.now = func() time.Time { return time.UnixMilli(1700000000000) }
	return c
}

func TestDoSignedAddsAuthAndSignature(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(context.Background())
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := signedTestClient(srv.URL)
	params := url.Values{}
	params.Set("symbol", "BTCUSDT")
	var out map[string]any
	if err := c.doSigned(context.Background(), http.MethodGet, "/api/v3/openOrders", params, &out); err != nil {
		t.Fatalf("doSigned: %v", err)
	}

	if got := captured.Header.Get("X-MBX-APIKEY"); got != "test-api-key" {
		t.Errorf("X-MBX-APIKEY = %q", got)
	}
	q := captured.URL.Query()
	if q.Get("timestamp") != "1700000000000" {
		t.Errorf("timestamp = %q", q.Get("timestamp"))
	}
	if q.Get("recvWindow") != "5000" {
		t.Errorf("recvWindow = %q", q.Get("recvWindow"))
	}

	// La signature doit couvrir exactement le payload envoyé (hors signature).
	values, _ := url.ParseQuery(captured.URL.RawQuery)
	sig := values.Get("signature")
	values.Del("signature")
	if want := sign(values.Encode(), "test-secret"); sig != want {
		t.Errorf("signature = %s, attendu %s", sig, want)
	}
}

func TestDoSignedRequiresKeys(t *testing.T) {
	c := NewClient(config.BinanceConfig{
		RESTBaseURL:        "http://example.invalid",
		RecvWindow:         time.Second,
		RateLimitCapacity:  1,
		RateLimitRefillPer: 1,
	}, slog.New(slog.DiscardHandler))

	if err := c.doSigned(context.Background(), http.MethodGet, "/x", nil, nil); err == nil {
		t.Fatal("sans clés API, doSigned doit échouer immédiatement")
	}
}

func TestExecuteRateLimitPenalty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
		w.Write([]byte(`{"code":-1003,"msg":"Too many requests."}`))
	}))
	defer srv.Close()

	c := signedTestClient(srv.URL)
	// Horloge contrôlée partagée avec le bucket.
	base := time.UnixMilli(1700000000000)
	c.limiter.now = func() time.Time { return base }
	c.limiter.last = base

	err := c.doSigned(context.Background(), http.MethodGet, "/api/v3/account", nil, nil)
	if !IsRateLimited(err) {
		t.Fatalf("attendu une erreur rate-limit, obtenu %v", err)
	}

	// Le bucket doit être gelé pendant ~7s (Retry-After).
	if delay, ok := c.limiter.tryTake(); ok || delay < 6*time.Second {
		t.Errorf("pénalité non appliquée (ok=%v, delay=%v)", ok, delay)
	}
}

func TestAPIErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"code":-2011,"msg":"Unknown order sent."}`))
	}))
	defer srv.Close()

	c := signedTestClient(srv.URL)
	err := c.doSigned(context.Background(), http.MethodDelete, "/api/v3/order", nil, nil)
	if !IsUnknownOrder(err) {
		t.Fatalf("attendu code -2011, obtenu %v", err)
	}
}
