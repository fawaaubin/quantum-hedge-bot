package gateway

import (
	"testing"
	"time"
)

func TestParseCombinedTrade(t *testing.T) {
	raw := []byte(`{"stream":"btcusdt@trade","data":{"e":"trade","E":1700000000100,"s":"BTCUSDT","t":12345,"p":"60123.45","q":"0.0021","T":1700000000099,"m":true}}`)

	ev, err := parseCombined(raw)
	if err != nil {
		t.Fatalf("parseCombined: %v", err)
	}
	if ev.trade == nil || ev.depth != nil {
		t.Fatal("attendu un événement trade")
	}
	if ev.trade.Symbol != "BTCUSDT" || ev.trade.Price != 60123.45 || ev.trade.Quantity != 0.0021 {
		t.Errorf("tick incorrect: %+v", ev.trade)
	}
	want := time.UnixMilli(1700000000099).UTC()
	if !ev.trade.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, attendu %v", ev.trade.Timestamp, want)
	}
}

func TestParseCombinedDepth(t *testing.T) {
	raw := []byte(`{"stream":"btcusdt@depth@100ms","data":{"e":"depthUpdate","E":1700000000200,"s":"BTCUSDT","U":157,"u":160,"b":[["60000.00","1.5"],["59999.00","0"]],"a":[["60001.00","2.25"]]}}`)

	ev, err := parseCombined(raw)
	if err != nil {
		t.Fatalf("parseCombined: %v", err)
	}
	if ev.depth == nil || ev.trade != nil {
		t.Fatal("attendu un événement depth")
	}
	d := ev.depth
	if d.FirstUpdateID != 157 || d.FinalUpdateID != 160 {
		t.Errorf("séquences U/u incorrectes: %d/%d", d.FirstUpdateID, d.FinalUpdateID)
	}
	if len(d.Bids) != 2 || d.Bids[1].Quantity != 0 {
		t.Errorf("les quantités nulles (suppressions) doivent être conservées: %+v", d.Bids)
	}
	if len(d.Asks) != 1 || d.Asks[0].Price != 60001 || d.Asks[0].Quantity != 2.25 {
		t.Errorf("asks incorrects: %+v", d.Asks)
	}
}

func TestParseCombinedRawStream(t *testing.T) {
	// Message d'un flux brut (/ws/...) sans enveloppe combinée.
	raw := []byte(`{"e":"trade","E":1,"s":"BTCUSDT","t":1,"p":"1.0","q":"2.0","T":1,"m":false}`)
	ev, err := parseCombined(raw)
	if err != nil {
		t.Fatalf("parseCombined: %v", err)
	}
	if ev.trade == nil {
		t.Fatal("attendu un événement trade")
	}
}

func TestParseCombinedUnknownEvent(t *testing.T) {
	raw := []byte(`{"stream":"x","data":{"e":"kline"}}`)
	if _, err := parseCombined(raw); err == nil {
		t.Fatal("un type d'événement inconnu doit retourner une erreur")
	}
}

func TestParseCombinedBadPrice(t *testing.T) {
	raw := []byte(`{"stream":"x","data":{"e":"trade","p":"abc","q":"1"}}`)
	if _, err := parseCombined(raw); err == nil {
		t.Fatal("un prix invalide doit retourner une erreur")
	}
}
