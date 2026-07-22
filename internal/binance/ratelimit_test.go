package binance

import (
	"context"
	"testing"
	"time"
)

// fakeClock simule le temps pour tester le bucket sans attente réelle.
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newTestBucket(capacity, refill float64) (*tokenBucket, *fakeClock) {
	clock := &fakeClock{t: time.UnixMilli(1700000000000)}
	b := newTokenBucket(capacity, refill)
	b.now = clock.now
	b.last = clock.t
	b.tokens = capacity
	return b, clock
}

func TestTokenBucketCapacity(t *testing.T) {
	b, _ := newTestBucket(3, 1)

	for i := 0; i < 3; i++ {
		if _, ok := b.tryTake(); !ok {
			t.Fatalf("le jeton %d doit être disponible", i+1)
		}
	}
	if delay, ok := b.tryTake(); ok || delay <= 0 {
		t.Fatalf("bucket vide: attendu un délai positif (ok=%v, delay=%v)", ok, delay)
	}
}

func TestTokenBucketRefill(t *testing.T) {
	b, clock := newTestBucket(2, 2) // 2 jetons/s
	b.tryTake()
	b.tryTake()

	if _, ok := b.tryTake(); ok {
		t.Fatal("bucket vide, prise impossible")
	}
	clock.advance(500 * time.Millisecond) // recharge 1 jeton
	if _, ok := b.tryTake(); !ok {
		t.Fatal("après 500ms à 2 jetons/s, un jeton doit être disponible")
	}
}

func TestTokenBucketPenalty(t *testing.T) {
	b, clock := newTestBucket(10, 10)

	b.penalize(5 * time.Second)
	delay, ok := b.tryTake()
	if ok {
		t.Fatal("pendant la pénalité, aucune prise possible")
	}
	if delay < 4*time.Second || delay > 5*time.Second {
		t.Errorf("délai de pénalité = %v, attendu ~5s", delay)
	}

	// Une pénalité plus courte ne raccourcit pas la pénalité en cours.
	b.penalize(time.Second)
	if d, _ := b.tryTake(); d < 4*time.Second {
		t.Errorf("la pénalité en cours a été raccourcie: %v", d)
	}

	clock.advance(6 * time.Second)
	if _, ok := b.tryTake(); !ok {
		t.Fatal("après la pénalité, la prise doit réussir")
	}
}

func TestTokenBucketWaitCancellation(t *testing.T) {
	b := newTokenBucket(1, 0.001) // recharge quasi nulle
	b.tryTake()                   // vide le bucket

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := b.wait(ctx); err == nil {
		t.Fatal("wait doit échouer à l'annulation du contexte")
	}
}
