package engine

import (
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Candle est une bougie OHLCV agrégée depuis les trades.
type Candle struct {
	Start  time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// candleAggregator agrège les ticks de trade en bougies d'intervalle
// fixe. Update retourne la bougie précédente lorsqu'elle se clôture
// (premier tick d'un nouvel intervalle), nil sinon.
type candleAggregator struct {
	interval time.Duration
	current  *Candle
}

func newCandleAggregator(interval time.Duration) *candleAggregator {
	return &candleAggregator{interval: interval}
}

func (a *candleAggregator) Update(t types.Tick) *Candle {
	bucket := t.Timestamp.Truncate(a.interval)

	if a.current == nil {
		a.current = &Candle{
			Start: bucket,
			Open:  t.Price, High: t.Price, Low: t.Price, Close: t.Price,
			Volume: t.Quantity,
		}
		return nil
	}

	if bucket.After(a.current.Start) {
		closed := *a.current
		a.current = &Candle{
			Start: bucket,
			Open:  t.Price, High: t.Price, Low: t.Price, Close: t.Price,
			Volume: t.Quantity,
		}
		return &closed
	}

	c := a.current
	if t.Price > c.High {
		c.High = t.Price
	}
	if t.Price < c.Low {
		c.Low = t.Price
	}
	c.Close = t.Price
	c.Volume += t.Quantity
	return nil
}
