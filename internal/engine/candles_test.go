package engine

import (
	"testing"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

func tick(ts time.Time, price, qty float64) types.Tick {
	return types.Tick{Symbol: "BTCUSDT", Timestamp: ts, Price: price, Quantity: qty}
}

func TestCandleAggregation(t *testing.T) {
	agg := newCandleAggregator(time.Minute)
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	// Trois ticks dans la même minute.
	if c := agg.Update(tick(t0.Add(1*time.Second), 100, 1)); c != nil {
		t.Fatal("pas de clôture au premier tick")
	}
	if c := agg.Update(tick(t0.Add(20*time.Second), 105, 2)); c != nil {
		t.Fatal("pas de clôture dans le même intervalle")
	}
	if c := agg.Update(tick(t0.Add(50*time.Second), 98, 1)); c != nil {
		t.Fatal("pas de clôture dans le même intervalle")
	}

	// Premier tick de la minute suivante : clôture de la bougie.
	closed := agg.Update(tick(t0.Add(61*time.Second), 99, 1))
	if closed == nil {
		t.Fatal("le changement d'intervalle doit clôturer la bougie")
	}
	if closed.Open != 100 || closed.High != 105 || closed.Low != 98 || closed.Close != 98 {
		t.Errorf("OHLC = %+v, attendu O=100 H=105 L=98 C=98", closed)
	}
	if closed.Volume != 4 {
		t.Errorf("volume = %v, attendu 4", closed.Volume)
	}
	if !closed.Start.Equal(t0) {
		t.Errorf("start = %v, attendu %v", closed.Start, t0)
	}
}

func TestCandleGapBetweenIntervals(t *testing.T) {
	// Un trou de plusieurs minutes clôture simplement la bougie en cours.
	agg := newCandleAggregator(time.Minute)
	t0 := time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC)

	agg.Update(tick(t0, 100, 1))
	closed := agg.Update(tick(t0.Add(5*time.Minute), 110, 1))
	if closed == nil || closed.Close != 100 {
		t.Fatalf("bougie clôturée attendue avec close=100, obtenu %+v", closed)
	}
}
