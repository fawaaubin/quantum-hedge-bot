package gateway

import (
	"testing"
	"time"
)

func TestNextBackoffBounds(t *testing.T) {
	initial := 500 * time.Millisecond
	max := 30 * time.Second

	for attempt := 0; attempt < 20; attempt++ {
		// base attendue : initial * 2^attempt, bornée par max.
		base := initial
		for i := 0; i < attempt && base < max; i++ {
			base *= 2
		}
		if base > max {
			base = max
		}

		for i := 0; i < 50; i++ {
			d := nextBackoff(attempt, initial, max)
			if d < base/2 || d > base {
				t.Fatalf("attempt=%d: délai %v hors bornes [%v, %v]", attempt, d, base/2, base)
			}
		}
	}
}

func TestNextBackoffRespectsMax(t *testing.T) {
	max := 2 * time.Second
	for i := 0; i < 100; i++ {
		if d := nextBackoff(50, time.Second, max); d > max {
			t.Fatalf("délai %v dépasse le maximum %v", d, max)
		}
	}
}

func TestNextBackoffDefensiveInputs(t *testing.T) {
	// Entrées incohérentes : ne doit ni paniquer ni retourner un délai nul.
	if d := nextBackoff(0, 0, 0); d <= 0 {
		t.Fatalf("délai non positif avec entrées nulles: %v", d)
	}
	if d := nextBackoff(3, time.Second, 100*time.Millisecond); d <= 0 {
		t.Fatalf("délai non positif avec max < initial: %v", d)
	}
}
