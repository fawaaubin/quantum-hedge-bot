package engine

import (
	"math"
	"testing"
)

func TestEMAKnownValues(t *testing.T) {
	// EMA(3) de [1,2,3,4,5] : amorce SMA3 = 2 ; k = 0,5 → 3 puis 4.
	e := NewEMA(3)
	inputs := []float64{1, 2, 3, 4, 5}
	want := []float64{0, 0, 2, 3, 4}

	for i, v := range inputs {
		e.Update(v)
		if got := e.Value(); math.Abs(got-want[i]) > 1e-9 {
			t.Errorf("après %v: EMA = %v, attendu %v", v, got, want[i])
		}
	}
	if !e.Ready() {
		t.Error("EMA doit être prête après 3 valeurs")
	}
}

func TestEMANotReadyBeforePeriod(t *testing.T) {
	e := NewEMA(9)
	for i := 0; i < 8; i++ {
		e.Update(100)
		if e.Ready() {
			t.Fatalf("EMA prête trop tôt (après %d valeurs)", i+1)
		}
	}
	e.Update(100)
	if !e.Ready() || e.Value() != 100 {
		t.Errorf("EMA(9) de 9×100 = %v, attendu 100", e.Value())
	}
}

func TestRSIWilderReference(t *testing.T) {
	// Jeu de données de référence (Wilder / StockCharts) : le premier
	// RSI(14) attendu est ≈ 70,46.
	closes := []float64{
		44.3389, 44.0902, 44.1497, 43.6124, 44.3278,
		44.8264, 45.0955, 45.4245, 45.8433, 46.0826,
		45.8931, 46.0328, 45.6140, 46.2820, 46.2820,
	}
	r := NewRSI(14)
	for i, c := range closes {
		r.Update(c)
		if i < 14 && r.Ready() {
			t.Fatalf("RSI prêt trop tôt (index %d)", i)
		}
	}
	if !r.Ready() {
		t.Fatal("RSI doit être prêt après 15 valeurs (14 variations)")
	}
	if got := r.Value(); math.Abs(got-70.46) > 0.1 {
		t.Errorf("RSI = %.4f, attendu ≈ 70.46", got)
	}
}

func TestRSIExtremes(t *testing.T) {
	// Hausses uniquement → RSI = 100.
	r := NewRSI(14)
	for i := 0; i <= 15; i++ {
		r.Update(float64(100 + i))
	}
	if got := r.Value(); got != 100 {
		t.Errorf("RSI en hausse pure = %v, attendu 100", got)
	}

	// Baisses uniquement → RSI proche de 0.
	r = NewRSI(14)
	for i := 0; i <= 15; i++ {
		r.Update(float64(100 - i))
	}
	if got := r.Value(); got > 1 {
		t.Errorf("RSI en baisse pure = %v, attendu ≈ 0", got)
	}
}
