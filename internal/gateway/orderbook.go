package gateway

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// errGap signale une discontinuité de séquence : le carnet local n'est
// plus fiable et doit être reconstruit depuis un nouveau snapshot REST.
var errGap = errors.New("désynchronisation du carnet: gap de séquence")

// errNotSynced signale qu'aucun snapshot n'a encore été appliqué.
var errNotSynced = errors.New("carnet non synchronisé")

// localBook maintient le carnet d'ordres local reconstruit à partir d'un
// snapshot REST et des mises à jour incrémentales du flux depth.
//
// Invariants : bids triés par prix décroissant, asks par prix croissant,
// aucun niveau de quantité nulle.
type localBook struct {
	mu           sync.RWMutex
	symbol       string
	synced       bool
	lastUpdateID int64
	bids         []types.PriceLevel
	asks         []types.PriceLevel
	updatedAt    time.Time
}

func newLocalBook(symbol string) *localBook {
	return &localBook{symbol: symbol}
}

// reset invalide le carnet (avant une resynchronisation).
func (b *localBook) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.synced = false
	b.lastUpdateID = 0
	b.bids = nil
	b.asks = nil
}

// applySnapshot remplace intégralement le carnet par le snapshot REST.
func (b *localBook) applySnapshot(ob types.OrderBook) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bids = sortLevels(ob.Bids, true)
	b.asks = sortLevels(ob.Asks, false)
	b.lastUpdateID = ob.LastUpdateID
	b.updatedAt = time.Now()
	b.synced = true
}

// applyDiff applique une mise à jour incrémentale en vérifiant la
// continuité des séquences (règle Binance Spot) :
//
//   - u <= lastUpdateID           : événement obsolète, ignoré ;
//   - U >  lastUpdateID+1         : gap, resynchronisation requise ;
//   - U <= lastUpdateID+1 <= u    : application (couvre le premier
//     événement après snapshot et les suivants contigus).
//
// Retourne (true, nil) si l'update a été appliquée, (false, nil) si elle
// est obsolète, (false, errGap) en cas de désynchronisation.
func (b *localBook) applyDiff(d types.DepthUpdate) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.synced {
		return false, errNotSynced
	}
	if d.FinalUpdateID <= b.lastUpdateID {
		return false, nil
	}
	if d.FirstUpdateID > b.lastUpdateID+1 {
		return false, errGap
	}

	for _, l := range d.Bids {
		b.bids = upsertLevel(b.bids, l, true)
	}
	for _, l := range d.Asks {
		b.asks = upsertLevel(b.asks, l, false)
	}
	b.lastUpdateID = d.FinalUpdateID
	b.updatedAt = d.EventTime
	return true, nil
}

// isSynced indique si un snapshot est appliqué et le carnet fiable.
func (b *localBook) isSynced() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.synced
}

// snapshot retourne une copie de l'état courant du carnet.
func (b *localBook) snapshot() types.OrderBook {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ob := types.OrderBook{
		Symbol:       b.symbol,
		LastUpdateID: b.lastUpdateID,
		Bids:         make([]types.PriceLevel, len(b.bids)),
		Asks:         make([]types.PriceLevel, len(b.asks)),
		UpdatedAt:    b.updatedAt,
	}
	copy(ob.Bids, b.bids)
	copy(ob.Asks, b.asks)
	return ob
}

// top retourne le meilleur bid, le meilleur ask et les volumes cumulés
// sur les n premiers niveaux de chaque côté.
func (b *localBook) top(n int) (bestBid, bestAsk types.PriceLevel, bidVol, askVol float64, ok bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.synced || len(b.bids) == 0 || len(b.asks) == 0 {
		return types.PriceLevel{}, types.PriceLevel{}, 0, 0, false
	}
	bestBid, bestAsk = b.bids[0], b.asks[0]
	for i := 0; i < n && i < len(b.bids); i++ {
		bidVol += b.bids[i].Quantity
	}
	for i := 0; i < n && i < len(b.asks); i++ {
		askVol += b.asks[i].Quantity
	}
	return bestBid, bestAsk, bidVol, askVol, true
}

// lastID retourne le dernier update ID appliqué.
func (b *localBook) lastID() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastUpdateID
}

// upsertLevel insère, remplace ou supprime (quantité nulle) un niveau en
// conservant l'ordre de tri (desc pour les bids, asc pour les asks).
func upsertLevel(levels []types.PriceLevel, l types.PriceLevel, desc bool) []types.PriceLevel {
	i := sort.Search(len(levels), func(i int) bool {
		if desc {
			return levels[i].Price <= l.Price
		}
		return levels[i].Price >= l.Price
	})
	if i < len(levels) && levels[i].Price == l.Price {
		if l.Quantity == 0 {
			return append(levels[:i], levels[i+1:]...)
		}
		levels[i].Quantity = l.Quantity
		return levels
	}
	if l.Quantity == 0 {
		return levels
	}
	levels = append(levels, types.PriceLevel{})
	copy(levels[i+1:], levels[i:])
	levels[i] = l
	return levels
}

// sortLevels retourne une copie triée en éliminant les quantités nulles.
func sortLevels(in []types.PriceLevel, desc bool) []types.PriceLevel {
	out := make([]types.PriceLevel, 0, len(in))
	for _, l := range in {
		if l.Quantity > 0 {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if desc {
			return out[i].Price > out[j].Price
		}
		return out[i].Price < out[j].Price
	})
	return out
}
