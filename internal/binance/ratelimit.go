package binance

import (
	"context"
	"sync"
	"time"
)

// tokenBucket est un rate limiter à jetons avec pénalité adaptative :
// une réponse 429/418 impose un gel jusqu'à l'échéance indiquée par
// l'exchange (Retry-After), pendant lequel toute prise de jeton attend.
type tokenBucket struct {
	mu           sync.Mutex
	capacity     float64
	tokens       float64
	refillPerSec float64
	last         time.Time
	penaltyUntil time.Time
	now          func() time.Time // injectable pour les tests
}

func newTokenBucket(capacity, refillPerSec float64) *tokenBucket {
	if capacity < 1 {
		capacity = 1
	}
	if refillPerSec <= 0 {
		refillPerSec = 1
	}
	b := &tokenBucket{
		capacity:     capacity,
		tokens:       capacity,
		refillPerSec: refillPerSec,
		now:          time.Now,
	}
	b.last = b.now()
	return b
}

// wait bloque jusqu'à obtention d'un jeton (et la fin de toute pénalité),
// ou jusqu'à annulation du contexte.
func (b *tokenBucket) wait(ctx context.Context) error {
	for {
		delay, ok := b.tryTake()
		if ok {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// tryTake tente de prendre un jeton. En cas d'échec, retourne le délai
// d'attente estimé avant nouvelle tentative.
func (b *tokenBucket) tryTake() (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	if now.Before(b.penaltyUntil) {
		return b.penaltyUntil.Sub(now), false
	}

	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * b.refillPerSec
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return 0, true
	}
	missing := 1 - b.tokens
	return time.Duration(missing / b.refillPerSec * float64(time.Second)), false
}

// penalize gèle le bucket pendant d (réponse 429/418). Une pénalité
// plus courte qu'une pénalité en cours est ignorée.
func (b *tokenBucket) penalize(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	until := b.now().Add(d)
	if until.After(b.penaltyUntil) {
		b.penaltyUntil = until
	}
}
