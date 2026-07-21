package gateway

import (
	"errors"
	"testing"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

func snapshot(lastID int64) types.OrderBook {
	return types.OrderBook{
		Symbol:       "BTCUSDT",
		LastUpdateID: lastID,
		Bids: []types.PriceLevel{
			{Price: 100, Quantity: 1},
			{Price: 99, Quantity: 2},
			{Price: 98, Quantity: 3},
		},
		Asks: []types.PriceLevel{
			{Price: 101, Quantity: 1},
			{Price: 102, Quantity: 2},
			{Price: 103, Quantity: 3},
		},
	}
}

func TestApplySnapshotSortsAndSyncs(t *testing.T) {
	b := newLocalBook("BTCUSDT")
	unsorted := types.OrderBook{
		LastUpdateID: 10,
		Bids: []types.PriceLevel{
			{Price: 98, Quantity: 3},
			{Price: 100, Quantity: 1},
			{Price: 99, Quantity: 0}, // quantité nulle : éliminée
		},
		Asks: []types.PriceLevel{
			{Price: 103, Quantity: 3},
			{Price: 101, Quantity: 1},
		},
	}
	b.applySnapshot(unsorted)

	if !b.isSynced() {
		t.Fatal("le carnet doit être synchronisé après snapshot")
	}
	ob := b.snapshot()
	if len(ob.Bids) != 2 || ob.Bids[0].Price != 100 || ob.Bids[1].Price != 98 {
		t.Errorf("bids mal triés ou mal filtrés: %+v", ob.Bids)
	}
	if len(ob.Asks) != 2 || ob.Asks[0].Price != 101 {
		t.Errorf("asks mal triés: %+v", ob.Asks)
	}
}

func TestApplyDiffNotSynced(t *testing.T) {
	b := newLocalBook("BTCUSDT")
	_, err := b.applyDiff(types.DepthUpdate{FirstUpdateID: 1, FinalUpdateID: 2})
	if !errors.Is(err, errNotSynced) {
		t.Fatalf("attendu errNotSynced, obtenu %v", err)
	}
}

func TestApplyDiffStaleSkipped(t *testing.T) {
	b := newLocalBook("BTCUSDT")
	b.applySnapshot(snapshot(100))

	applied, err := b.applyDiff(types.DepthUpdate{FirstUpdateID: 90, FinalUpdateID: 100})
	if err != nil || applied {
		t.Fatalf("un événement obsolète doit être ignoré (applied=%v, err=%v)", applied, err)
	}
	if b.lastID() != 100 {
		t.Errorf("lastUpdateID modifié par un événement obsolète: %d", b.lastID())
	}
}

func TestApplyDiffOverlapFirstEvent(t *testing.T) {
	// Premier événement après snapshot : U <= lastUpdateID+1 <= u.
	b := newLocalBook("BTCUSDT")
	b.applySnapshot(snapshot(100))

	applied, err := b.applyDiff(types.DepthUpdate{
		FirstUpdateID: 95,
		FinalUpdateID: 105,
		Bids:          []types.PriceLevel{{Price: 100.5, Quantity: 5}},
	})
	if err != nil || !applied {
		t.Fatalf("l'événement chevauchant doit être appliqué (applied=%v, err=%v)", applied, err)
	}
	if b.lastID() != 105 {
		t.Errorf("lastUpdateID = %d, attendu 105", b.lastID())
	}
	ob := b.snapshot()
	if ob.Bids[0].Price != 100.5 || ob.Bids[0].Quantity != 5 {
		t.Errorf("nouveau meilleur bid non appliqué: %+v", ob.Bids[0])
	}
}

func TestApplyDiffContiguous(t *testing.T) {
	b := newLocalBook("BTCUSDT")
	b.applySnapshot(snapshot(100))

	if _, err := b.applyDiff(types.DepthUpdate{FirstUpdateID: 101, FinalUpdateID: 110}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.applyDiff(types.DepthUpdate{FirstUpdateID: 111, FinalUpdateID: 120}); err != nil {
		t.Fatal(err)
	}
	if b.lastID() != 120 {
		t.Errorf("lastUpdateID = %d, attendu 120", b.lastID())
	}
}

func TestApplyDiffGap(t *testing.T) {
	b := newLocalBook("BTCUSDT")
	b.applySnapshot(snapshot(100))

	_, err := b.applyDiff(types.DepthUpdate{FirstUpdateID: 150, FinalUpdateID: 160})
	if !errors.Is(err, errGap) {
		t.Fatalf("un gap de séquence doit retourner errGap, obtenu %v", err)
	}

	// Après resynchronisation (reset + nouveau snapshot), le carnet
	// doit accepter à nouveau des événements.
	b.reset()
	if b.isSynced() {
		t.Fatal("reset doit invalider le carnet")
	}
	b.applySnapshot(snapshot(200))
	applied, err := b.applyDiff(types.DepthUpdate{FirstUpdateID: 201, FinalUpdateID: 210})
	if err != nil || !applied {
		t.Fatalf("le carnet resynchronisé doit accepter les événements (applied=%v, err=%v)", applied, err)
	}
}

func TestApplyDiffRemovesZeroQuantityLevels(t *testing.T) {
	b := newLocalBook("BTCUSDT")
	b.applySnapshot(snapshot(100))

	applied, err := b.applyDiff(types.DepthUpdate{
		FirstUpdateID: 101,
		FinalUpdateID: 102,
		Bids:          []types.PriceLevel{{Price: 100, Quantity: 0}}, // suppression du meilleur bid
		Asks:          []types.PriceLevel{{Price: 101, Quantity: 0}}, // suppression du meilleur ask
	})
	if err != nil || !applied {
		t.Fatal(err)
	}
	ob := b.snapshot()
	if ob.Bids[0].Price != 99 {
		t.Errorf("meilleur bid = %v, attendu 99", ob.Bids[0].Price)
	}
	if ob.Asks[0].Price != 102 {
		t.Errorf("meilleur ask = %v, attendu 102", ob.Asks[0].Price)
	}
}

func TestTopVolumes(t *testing.T) {
	b := newLocalBook("BTCUSDT")
	b.applySnapshot(snapshot(100))

	bid, ask, bidVol, askVol, ok := b.top(20)
	if !ok {
		t.Fatal("top doit réussir sur un carnet synchronisé")
	}
	if bid.Price != 100 || ask.Price != 101 {
		t.Errorf("best bid/ask = %v/%v, attendu 100/101", bid.Price, ask.Price)
	}
	if bidVol != 6 || askVol != 6 {
		t.Errorf("volumes cumulés = %v/%v, attendu 6/6", bidVol, askVol)
	}
}

func TestUpsertLevelKeepsOrder(t *testing.T) {
	var bids []types.PriceLevel
	for _, p := range []float64{99, 101, 100, 98.5} {
		bids = upsertLevel(bids, types.PriceLevel{Price: p, Quantity: 1}, true)
	}
	want := []float64{101, 100, 99, 98.5}
	for i, w := range want {
		if bids[i].Price != w {
			t.Fatalf("bids[%d] = %v, attendu %v (ordre décroissant)", i, bids[i].Price, w)
		}
	}

	// Remplacement d'une quantité existante.
	bids = upsertLevel(bids, types.PriceLevel{Price: 100, Quantity: 7}, true)
	if len(bids) != 4 || bids[1].Quantity != 7 {
		t.Errorf("remplacement de quantité incorrect: %+v", bids)
	}

	// Suppression d'un niveau inexistant : sans effet.
	bids = upsertLevel(bids, types.PriceLevel{Price: 42, Quantity: 0}, true)
	if len(bids) != 4 {
		t.Errorf("la suppression d'un niveau absent ne doit rien changer: %+v", bids)
	}
}
