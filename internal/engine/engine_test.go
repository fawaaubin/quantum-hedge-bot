package engine

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

func engineConfig() config.EngineConfig {
	return config.EngineConfig{
		CandleInterval:  time.Minute,
		EMAFastPeriod:   9,
		EMASlowPeriod:   21,
		RSIPeriod:       14,
		RSIEntry:        55,
		RSIExit:         45,
		MaxSpreadPct:    0.0005,
		MinLiquidityBTC: 2.0,
	}
}

func newTestEngine(trailing TrailingUpdater, breaker BreakerCheck) *Engine {
	return New(engineConfig(), "BTCUSDT", trailing, breaker, slog.New(slog.DiscardHandler))
}

// goodTick fabrique un tick avec un carnet sain (spread étroit,
// liquidité suffisante).
func goodTick(ts time.Time, price float64) types.MarketEvent {
	return types.MarketEvent{Kind: types.EventTrade, Tick: &types.Tick{
		Symbol: "BTCUSDT", Timestamp: ts, Price: price, Quantity: 0.1,
		BestBid: price - 0.5, BestAsk: price + 0.5, // spread « 1 » sur ~60000
		BidVolume: 3, AskVolume: 3,
	}}
}

// feedCandles envoie un tick par intervalle (chaque tick clôture la
// bougie précédente) et collecte les signaux émis.
func feedCandles(t *testing.T, e *Engine, start time.Time, closes []float64) []types.Signal {
	t.Helper()
	var signals []types.Signal
	for i, c := range closes {
		sigs, err := e.OnEvent(context.Background(), goodTick(start.Add(time.Duration(i)*time.Minute), c))
		if err != nil {
			t.Fatal(err)
		}
		signals = append(signals, sigs...)
	}
	return signals
}

// crossScenario produit une série de closes : baisse douce (EMA rapide
// sous la lente) puis rallye marqué (croisement haussier + RSI élevé).
func crossScenario() []float64 {
	var closes []float64
	price := 60000.0
	for i := 0; i < 30; i++ { // tendance baissière douce
		price -= 20
		closes = append(closes, price)
	}
	for i := 0; i < 12; i++ { // rallye
		price += 120
		closes = append(closes, price)
	}
	return closes
}

func TestEntrySignalOnBullishCross(t *testing.T) {
	e := newTestEngine(nil, nil)
	signals := feedCandles(t, e, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), crossScenario())

	if len(signals) == 0 {
		t.Fatal("le scénario de croisement haussier doit émettre un signal d'entrée")
	}
	sig := signals[0]
	if sig.Action != types.ActionEnterLong {
		t.Errorf("action = %s, attendu ENTER_LONG", sig.Action)
	}
	if sig.Reason != "ema_cross_up" {
		t.Errorf("reason = %s", sig.Reason)
	}
	if sig.Price <= 0 {
		t.Error("le signal doit porter le meilleur bid comme prix de placement")
	}
	// Le moteur reste IDLE : c'est l'orchestrateur qui pilote l'état.
	if e.State() != types.StateIdle {
		t.Errorf("état = %s, attendu IDLE", e.State())
	}
}

func TestNoEntryWhenSpreadTooWide(t *testing.T) {
	e := newTestEngine(nil, nil)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closes := crossScenario()

	var signals []types.Signal
	for i, c := range closes {
		ev := goodTick(start.Add(time.Duration(i)*time.Minute), c)
		ev.Tick.BestAsk = ev.Tick.BestBid * 1.001 // spread 0,1 % > 0,05 %
		sigs, err := e.OnEvent(context.Background(), ev)
		if err != nil {
			t.Fatal(err)
		}
		signals = append(signals, sigs...)
	}
	if len(signals) != 0 {
		t.Errorf("spread trop large: aucun signal attendu, obtenu %+v", signals)
	}
}

func TestNoEntryWhenLiquidityTooLow(t *testing.T) {
	e := newTestEngine(nil, nil)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var signals []types.Signal
	for i, c := range crossScenario() {
		ev := goodTick(start.Add(time.Duration(i)*time.Minute), c)
		ev.Tick.BidVolume, ev.Tick.AskVolume = 0.5, 0.5 // 1 BTC < 2 BTC
		sigs, err := e.OnEvent(context.Background(), ev)
		if err != nil {
			t.Fatal(err)
		}
		signals = append(signals, sigs...)
	}
	if len(signals) != 0 {
		t.Errorf("liquidité insuffisante: aucun signal attendu, obtenu %+v", signals)
	}
}

func TestNoEntryWhenBreakerActive(t *testing.T) {
	e := newTestEngine(nil, func() bool { return true })
	signals := feedCandles(t, e, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), crossScenario())
	if len(signals) != 0 {
		t.Errorf("breaker actif: aucun signal attendu, obtenu %+v", signals)
	}
}

func TestStateMachineTransitions(t *testing.T) {
	e := newTestEngine(nil, nil)

	if e.State() != types.StateIdle {
		t.Fatalf("état initial = %s", e.State())
	}
	e.EntryPlaced()
	if e.State() != types.StateWaitingEntry {
		t.Fatalf("après EntryPlaced: %s", e.State())
	}
	e.EntryFilled(types.Position{AvgEntry: 60000, StopLoss: 59400, TakeProfit: 61200})
	if e.State() != types.StateInPosition {
		t.Fatalf("après EntryFilled: %s", e.State())
	}
	e.ExitPlaced()
	if e.State() != types.StateWaitingExit {
		t.Fatalf("après ExitPlaced: %s", e.State())
	}
	e.Flat()
	if e.State() != types.StateIdle {
		t.Fatalf("après Flat: %s", e.State())
	}
	if e.Position().AvgEntry != 0 {
		t.Error("Flat doit purger la position")
	}
}

func inPositionEngine(trailing TrailingUpdater, breaker BreakerCheck) *Engine {
	e := newTestEngine(trailing, breaker)
	e.EntryPlaced()
	e.EntryFilled(types.Position{
		Symbol: "BTCUSDT", Side: types.SideBuy, Quantity: 0.01,
		AvgEntry: 60000, StopLoss: 59400, TakeProfit: 61200,
	})
	return e
}

func exitOn(t *testing.T, e *Engine, price float64, wantReason string) {
	t.Helper()
	sigs, err := e.OnEvent(context.Background(),
		goodTick(time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC), price))
	if err != nil {
		t.Fatal(err)
	}
	if len(sigs) != 1 || sigs[0].Action != types.ActionExit || sigs[0].Reason != wantReason {
		t.Fatalf("attendu sortie %q, obtenu %+v", wantReason, sigs)
	}
}

func TestExitOnStopLoss(t *testing.T) {
	exitOn(t, inPositionEngine(nil, nil), 59399, "stop_loss")
}

func TestExitOnTakeProfit(t *testing.T) {
	exitOn(t, inPositionEngine(nil, nil), 61300, "take_profit")
}

func TestExitOnTrailingStop(t *testing.T) {
	trailing := func(pos types.Position, price float64) (float64, bool) {
		return 60500, price <= 60500
	}
	exitOn(t, inPositionEngine(trailing, nil), 60400, "trailing_stop")
}

func TestExitOnBreaker(t *testing.T) {
	exitOn(t, inPositionEngine(nil, func() bool { return true }), 60000, "circuit_breaker")
}

func TestTrailingStopStateIsPersisted(t *testing.T) {
	// Le stop progressif retourné par le trailing doit être conservé
	// dans la position entre deux ticks.
	calls := 0
	trailing := func(pos types.Position, price float64) (float64, bool) {
		calls++
		return pos.TrailingStop + 10, false
	}
	e := inPositionEngine(trailing, nil)

	ts := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	e.OnEvent(context.Background(), goodTick(ts, 60050))
	e.OnEvent(context.Background(), goodTick(ts.Add(time.Second), 60060))

	if calls != 2 {
		t.Fatalf("trailing appelé %d fois, attendu 2", calls)
	}
	if got := e.Position().TrailingStop; got != 20 {
		t.Errorf("trailing cumulé = %v, attendu 20 (10 par tick)", got)
	}
}

func TestExitOnBearishCrossAfterEntry(t *testing.T) {
	e := newTestEngine(nil, nil)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Rallye → signal d'entrée émis quelque part.
	closes := crossScenario()
	feedCandles(t, e, start, closes)
	e.EntryPlaced()
	e.EntryFilled(types.Position{AvgEntry: 60000, StopLoss: 1, TakeProfit: 10_000_000})

	// Chute marquée : croisement baissier ou RSI < 45 → signal de sortie.
	price := closes[len(closes)-1]
	var got []types.Signal
	for i := 1; i <= 25 && len(got) == 0; i++ {
		price -= 150
		sigs, err := e.OnEvent(context.Background(),
			goodTick(start.Add(time.Duration(len(closes)+i)*time.Minute), price))
		if err != nil {
			t.Fatal(err)
		}
		got = sigs
	}
	if len(got) == 0 {
		t.Fatal("la chute doit produire un signal de sortie (cross ou RSI)")
	}
	if got[0].Action != types.ActionExit {
		t.Errorf("action = %s, attendu EXIT", got[0].Action)
	}
	if r := got[0].Reason; r != "ema_cross_down" && r != "rsi_exit" {
		t.Errorf("reason = %s, attendu ema_cross_down ou rsi_exit", r)
	}
}

func TestDepthEventsIgnored(t *testing.T) {
	e := newTestEngine(nil, nil)
	sigs, err := e.OnEvent(context.Background(), types.MarketEvent{
		Kind:  types.EventDepth,
		Depth: &types.DepthUpdate{},
	})
	if err != nil || sigs != nil {
		t.Errorf("les événements depth ne produisent rien (sigs=%v, err=%v)", sigs, err)
	}
}
