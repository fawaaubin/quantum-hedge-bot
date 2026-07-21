package gateway

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

type fakeSnapshotter struct {
	book types.OrderBook
	err  error
}

func (f *fakeSnapshotter) DepthSnapshot(ctx context.Context, symbol string, limit int) (types.OrderBook, error) {
	return f.book, f.err
}

func testGateway(bufferSize int) *Gateway {
	cfg := config.GatewayConfig{
		EventBufferSize: bufferSize,
		BackoffInitial:  time.Millisecond,
		BackoffMax:      10 * time.Millisecond,
		SnapshotTimeout: time.Second,
		SnapshotDepth:   100,
	}
	log := slog.New(slog.DiscardHandler)
	return New(cfg, "wss://example.invalid", "BTCUSDT", &fakeSnapshotter{book: snapshot(100)}, log)
}

func TestHandleMessageDepthUpdatesBook(t *testing.T) {
	g := testGateway(16)
	g.book.applySnapshot(snapshot(100))

	raw := []byte(`{"stream":"btcusdt@depth@100ms","data":{"e":"depthUpdate","E":1,"s":"BTCUSDT","U":101,"u":102,"b":[["100.00","9"]],"a":[]}}`)
	if err := g.handleMessage(raw); err != nil {
		t.Fatalf("handleMessage: %v", err)
	}

	ob, err := g.OrderBook(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if ob.Bids[0].Quantity != 9 {
		t.Errorf("le carnet doit refléter l'update: %+v", ob.Bids[0])
	}

	select {
	case ev := <-g.Events():
		if ev.Kind != types.EventDepth || ev.Depth.FinalUpdateID != 102 {
			t.Errorf("événement depth incorrect: %+v", ev)
		}
	default:
		t.Fatal("un événement depth doit être publié")
	}
}

func TestHandleMessageGapPropagated(t *testing.T) {
	g := testGateway(16)
	g.book.applySnapshot(snapshot(100))

	raw := []byte(`{"stream":"s","data":{"e":"depthUpdate","E":1,"s":"BTCUSDT","U":500,"u":510,"b":[],"a":[]}}`)
	if err := g.handleMessage(raw); err == nil {
		t.Fatal("un gap doit remonter errGap pour déclencher la resynchronisation")
	}
}

func TestHandleMessageTradeEnrichedFromBook(t *testing.T) {
	g := testGateway(16)
	g.book.applySnapshot(snapshot(100))

	raw := []byte(`{"stream":"btcusdt@trade","data":{"e":"trade","E":1,"s":"BTCUSDT","t":1,"p":"100.5","q":"0.5","T":1,"m":false}}`)
	if err := g.handleMessage(raw); err != nil {
		t.Fatal(err)
	}

	ev := <-g.Events()
	if ev.Kind != types.EventTrade {
		t.Fatalf("attendu un trade, obtenu %+v", ev)
	}
	tick := ev.Tick
	if tick.BestBid != 100 || tick.BestAsk != 101 {
		t.Errorf("tick non enrichi du carnet: %+v", tick)
	}
	if tick.BidVolume != 6 || tick.AskVolume != 6 {
		t.Errorf("volumes cumulés incorrects: %+v", tick)
	}
	if tick.LastUpdateID != 100 {
		t.Errorf("LastUpdateID = %d, attendu 100", tick.LastUpdateID)
	}
}

func TestPublishDropsWhenFull(t *testing.T) {
	g := testGateway(1)

	g.publish(types.MarketEvent{Kind: types.EventTrade})
	g.publish(types.MarketEvent{Kind: types.EventTrade})

	if got := g.Dropped(); got != 1 {
		t.Errorf("dropped = %d, attendu 1 (channel plein, lecture jamais bloquée)", got)
	}
}

func TestOrderBookRequiresSync(t *testing.T) {
	g := testGateway(1)
	if _, err := g.OrderBook(context.Background(), "BTCUSDT"); err == nil {
		t.Fatal("un carnet non synchronisé doit retourner une erreur")
	}
	if _, err := g.OrderBook(context.Background(), "ETHUSDT"); err == nil {
		t.Fatal("un symbole inconnu doit retourner une erreur")
	}
}

func TestBootstrapUsesSnapshotFetcher(t *testing.T) {
	g := testGateway(16)
	if err := g.bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !g.book.isSynced() || g.book.lastID() != 100 {
		t.Errorf("bootstrap doit appliquer le snapshot (synced=%v, lastID=%d)",
			g.book.isSynced(), g.book.lastID())
	}
}
