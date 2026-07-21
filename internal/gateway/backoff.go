package gateway

import (
	"math/rand"
	"time"
)

// nextBackoff calcule le délai de reconnexion pour la tentative n
// (0-indexée) : backoff exponentiel borné par max, avec un jitter
// uniforme dans [base/2, base] pour désynchroniser les reconnexions.
func nextBackoff(attempt int, initial, max time.Duration) time.Duration {
	if initial <= 0 {
		initial = 500 * time.Millisecond
	}
	if max < initial {
		max = initial
	}

	base := initial
	for i := 0; i < attempt; i++ {
		base *= 2
		if base >= max {
			base = max
			break
		}
	}

	half := base / 2
	return half + time.Duration(rand.Int63n(int64(half)+1))
}
