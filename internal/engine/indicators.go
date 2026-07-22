package engine

// Indicateurs incrémentaux : chaque mise à jour est O(1), sans
// recalcul d'historique — indispensable en temps réel comme en backtest
// (déterminisme strict : mêmes entrées, mêmes sorties).

// EMA est une moyenne mobile exponentielle amorcée par la SMA des
// `period` premières valeurs (convention standard).
type EMA struct {
	period int
	k      float64
	value  float64
	count  int
	sum    float64
}

// NewEMA construit une EMA de période donnée.
func NewEMA(period int) *EMA {
	return &EMA{period: period, k: 2.0 / float64(period+1)}
}

// Update intègre une nouvelle valeur (close de bougie).
func (e *EMA) Update(v float64) {
	e.count++
	if e.count <= e.period {
		e.sum += v
		if e.count == e.period {
			e.value = e.sum / float64(e.period)
		}
		return
	}
	e.value = v*e.k + e.value*(1-e.k)
}

// Ready indique si l'amorce (SMA) est complète.
func (e *EMA) Ready() bool { return e.count >= e.period }

// Value retourne la valeur courante (0 tant que Ready() est faux).
func (e *EMA) Value() float64 {
	if !e.Ready() {
		return 0
	}
	return e.value
}

// RSI implémente le RSI de Wilder : première moyenne = moyenne simple
// des `period` premières variations, puis lissage de Wilder.
type RSI struct {
	period  int
	prev    float64
	count   int // nombre de valeurs reçues
	avgGain float64
	avgLoss float64
}

// NewRSI construit un RSI de période donnée.
func NewRSI(period int) *RSI {
	return &RSI{period: period}
}

// Update intègre un nouveau close.
func (r *RSI) Update(v float64) {
	r.count++
	if r.count == 1 {
		r.prev = v
		return
	}
	change := v - r.prev
	r.prev = v

	var gain, loss float64
	if change > 0 {
		gain = change
	} else {
		loss = -change
	}

	n := r.count - 1 // nombre de variations observées
	switch {
	case n < r.period:
		r.avgGain += gain
		r.avgLoss += loss
	case n == r.period:
		r.avgGain = (r.avgGain + gain) / float64(r.period)
		r.avgLoss = (r.avgLoss + loss) / float64(r.period)
	default:
		p := float64(r.period)
		r.avgGain = (r.avgGain*(p-1) + gain) / p
		r.avgLoss = (r.avgLoss*(p-1) + loss) / p
	}
}

// Ready indique si `period` variations ont été observées.
func (r *RSI) Ready() bool { return r.count-1 >= r.period }

// Value retourne le RSI dans [0, 100] (0 tant que Ready() est faux).
func (r *RSI) Value() float64 {
	if !r.Ready() {
		return 0
	}
	if r.avgLoss == 0 {
		return 100
	}
	rs := r.avgGain / r.avgLoss
	return 100 - 100/(1+rs)
}
