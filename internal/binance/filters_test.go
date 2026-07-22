package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

func btcFilters() types.SymbolFilters {
	return types.SymbolFilters{
		Symbol:        "BTCUSDT",
		StepSize:      0.00001,
		TickSize:      0.01,
		MinQty:        0.00001,
		MaxQty:        9000,
		MinNotional:   5,
		QtyDecimals:   5,
		PriceDecimals: 2,
	}
}

func TestValidateAndQuantize(t *testing.T) {
	req := types.OrderRequest{
		Symbol:   "BTCUSDT",
		Side:     types.SideBuy,
		Type:     types.OrderTypeLimit,
		Price:    60123.456789, // → 60123.45 (plancher tickSize)
		Quantity: 0.001234567,  // → 0.00123 (plancher stepSize)
	}
	got, err := ValidateAndQuantize(req, btcFilters())
	if err != nil {
		t.Fatalf("ValidateAndQuantize: %v", err)
	}
	if got.Price != 60123.45 {
		t.Errorf("prix quantifié = %v, attendu 60123.45", got.Price)
	}
	if got.Quantity != 0.00123 {
		t.Errorf("quantité quantifiée = %v, attendu 0.00123", got.Quantity)
	}
}

func TestValidateAndQuantizeRejections(t *testing.T) {
	f := btcFilters()

	cases := []struct {
		name string
		req  types.OrderRequest
		want string
	}{
		{"sous minQty", types.OrderRequest{Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.000001}, "minQty"},
		{"au-dessus maxQty", types.OrderRequest{Type: types.OrderTypeLimit, Price: 60000, Quantity: 10000}, "maxQty"},
		{"sous minNotional", types.OrderRequest{Type: types.OrderTypeLimit, Price: 100, Quantity: 0.001}, "minNotional"},
		{"prix nul", types.OrderRequest{Type: types.OrderTypeLimit, Price: 0, Quantity: 1}, "prix"},
	}
	for _, tc := range cases {
		if _, err := ValidateAndQuantize(tc.req, f); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: attendu une erreur contenant %q, obtenu %v", tc.name, tc.want, err)
		}
	}
}

func TestFormatQtyPrice(t *testing.T) {
	f := btcFilters()
	if got := FormatQty(f, 0.00123); got != "0.00123" {
		t.Errorf("FormatQty = %q", got)
	}
	if got := FormatPrice(f, 60123.45); got != "60123.45" {
		t.Errorf("FormatPrice = %q", got)
	}
}

func TestDecimalsOf(t *testing.T) {
	cases := map[string]int{
		"0.00001000": 5,
		"0.01000000": 2,
		"1.00000000": 0,
		"1":          0,
		"0.1":        1,
	}
	for in, want := range cases {
		if got := decimalsOf(in); got != want {
			t.Errorf("decimalsOf(%q) = %d, attendu %d", in, got, want)
		}
	}
}

func TestExchangeFiltersParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/exchangeInfo" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"symbols":[{"symbol":"BTCUSDT","filters":[
			{"filterType":"PRICE_FILTER","tickSize":"0.01000000"},
			{"filterType":"LOT_SIZE","stepSize":"0.00001000","minQty":"0.00001000","maxQty":"9000.00000000"},
			{"filterType":"NOTIONAL","minNotional":"5.00000000"}
		]}]}`))
	}))
	defer srv.Close()

	f, err := newTestClient(srv.URL).ExchangeFilters(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("ExchangeFilters: %v", err)
	}
	want := btcFilters()
	if f != want {
		t.Errorf("filtres = %+v, attendu %+v", f, want)
	}
}
