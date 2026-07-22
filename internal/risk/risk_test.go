package risk

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

func testConfig() config.RiskConfig {
	return config.RiskConfig{
		InitialCapital:    1000,
		MaxRiskPerTrade:   0.01,
		MaxAbsoluteRisk:   0.02,
		MaxOpenPositions:  1,
		BreakerLookback:   10,
		BreakerLossPct:    0.05,
		BreakerCooldown:   4 * time.Hour,
		StopLossPct:       0.01,
		TakeProfitPct:     0.02,
		TrailingActivePct: 0.01,
		TrailingStepPct:   0.005,
	}
}

func newTestManager(cfg config.RiskConfig) (*Manager, *time.Time) {
	m := New(cfg, slog.New(slog.DiscardHandler))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := &now
	m.now = func() time.Time { return *clock }
	return m, clock
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPositionSize(t *testing.T) {
	m, _ := newTestManager(testConfig())
	ctx := context.Background()

	// Risque 1 % de 1000 = 10 ; risque par unité = 100 - 99 = 1 → 10 unités.
	qty, err := m.PositionSize(ctx, 100, 99, 1000)
	if err != nil {
		t.Fatalf("PositionSize: %v", err)
	}
	if !almostEqual(qty, 10) {
		t.Errorf("qty = %v, attendu 10", qty)
	}
}

func TestPositionSizeCappedByCapital(t *testing.T) {
	m, _ := newTestManager(testConfig())

	// Stop très serré → taille théorique énorme, plafonnée sans levier :
	// capital/entry = 1000/100 = 10 unités max.
	qty, err := m.PositionSize(context.Background(), 100, 99.99, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !almostEqual(qty, 10) {
		t.Errorf("qty = %v, attendu le plafond capital/entry = 10", qty)
	}
}

func TestPositionSizeErrors(t *testing.T) {
	m, _ := newTestManager(testConfig())
	ctx := context.Background()

	if _, err := m.PositionSize(ctx, 100, 100, 1000); err == nil {
		t.Error("stop loss == entrée doit échouer")
	}
	if _, err := m.PositionSize(ctx, 100, 101, 1000); err == nil {
		t.Error("stop loss > entrée doit échouer (LONG uniquement)")
	}
	if _, err := m.PositionSize(ctx, 100, 99, 0); err == nil {
		t.Error("capital nul doit échouer")
	}
	if _, err := m.PositionSize(ctx, 0, 99, 1000); err == nil {
		t.Error("entrée nulle doit échouer")
	}
}

func validSignal() types.Signal {
	return types.Signal{
		Symbol: "BTCUSDT", Action: types.ActionEnterLong,
		Price: 60000, Timestamp: time.Now(),
	}
}

func TestValidateEntryAccepts(t *testing.T) {
	m, _ := newTestManager(testConfig())
	if err := m.ValidateEntry(context.Background(), validSignal()); err != nil {
		t.Fatalf("un signal valide doit passer: %v", err)
	}
}

func TestValidateEntryRejectsBadSignal(t *testing.T) {
	m, _ := newTestManager(testConfig())
	sig := validSignal()
	sig.Action = types.ActionExit
	if err := m.ValidateEntry(context.Background(), sig); !errors.Is(err, ErrInvalidSignal) {
		t.Errorf("attendu ErrInvalidSignal, obtenu %v", err)
	}
}

func TestValidateEntryMaxPositions(t *testing.T) {
	m, _ := newTestManager(testConfig())
	m.PositionOpened(10)

	if err := m.ValidateEntry(context.Background(), validSignal()); !errors.Is(err, ErrMaxPositions) {
		t.Errorf("attendu ErrMaxPositions, obtenu %v", err)
	}
}

func TestValidateEntryAbsoluteRisk(t *testing.T) {
	cfg := testConfig()
	cfg.MaxOpenPositions = 3    // ne pas bloquer sur le nombre de positions
	cfg.MaxAbsoluteRisk = 0.015 // 15 de risque max sur capital 1000
	m, _ := newTestManager(cfg)

	// Première position : 10 de risque engagé (1 %).
	m.PositionOpened(10)

	// Nouvelle entrée : 10 + 10 = 20 > 15 → refus.
	if err := m.ValidateEntry(context.Background(), validSignal()); !errors.Is(err, ErrMaxAbsoluteRisk) {
		t.Errorf("attendu ErrMaxAbsoluteRisk, obtenu %v", err)
	}
}

func TestExitLevels(t *testing.T) {
	m, _ := newTestManager(testConfig())

	levels, err := m.ExitLevels(context.Background(), types.Position{AvgEntry: 60000})
	if err != nil {
		t.Fatal(err)
	}
	if !almostEqual(levels.StopLoss, 59400) { // -1 %
		t.Errorf("SL = %v, attendu 59400", levels.StopLoss)
	}
	if !almostEqual(levels.TakeProfit, 61200) { // +2 %
		t.Errorf("TP = %v, attendu 61200", levels.TakeProfit)
	}
	if levels.TrailingStop != 0 {
		t.Errorf("trailing initial = %v, attendu 0 (inactif)", levels.TrailingStop)
	}

	if _, err := m.ExitLevels(context.Background(), types.Position{}); err == nil {
		t.Error("position sans prix moyen doit échouer")
	}
}

func TestUpdateTrailing(t *testing.T) {
	m, _ := newTestManager(testConfig())
	pos := types.Position{AvgEntry: 60000}

	// Sous +1 % : inactif.
	if stop, trig := m.UpdateTrailing(pos, 60500); stop != 0 || trig {
		t.Errorf("sous le seuil d'activation: stop=%v trig=%v", stop, trig)
	}

	// À +1 % (60600) : activation, stop = 60600 × 0.995 = 60297.
	stop, trig := m.UpdateTrailing(pos, 60600)
	if !almostEqual(stop, 60297) || trig {
		t.Errorf("activation: stop=%v (attendu 60297) trig=%v", stop, trig)
	}
	pos.TrailingStop = stop

	// Le prix monte : le stop suit (cliquet).
	stop, trig = m.UpdateTrailing(pos, 61000)
	if !almostEqual(stop, 61000*0.995) || trig {
		t.Errorf("progression: stop=%v trig=%v", stop, trig)
	}
	pos.TrailingStop = stop

	// Le prix redescend : le stop ne recule jamais.
	stop, _ = m.UpdateTrailing(pos, 60800)
	if !almostEqual(stop, 61000*0.995) {
		t.Errorf("le stop suiveur a reculé: %v", stop)
	}

	// Le prix casse le stop : déclenchement.
	if _, trig := m.UpdateTrailing(pos, 60600); !trig {
		t.Error("le passage sous le stop doit déclencher la sortie")
	}
}

func TestRecordTradeResultUpdatesCapital(t *testing.T) {
	m, _ := newTestManager(testConfig())
	m.PositionOpened(10)

	if err := m.RecordTradeResult(context.Background(), types.TradeResult{PnL: 25}); err != nil {
		t.Fatal(err)
	}
	if got := m.Capital(); !almostEqual(got, 1025) {
		t.Errorf("capital = %v, attendu 1025", got)
	}
	// La position et le risque engagé sont libérés.
	if err := m.ValidateEntry(context.Background(), validSignal()); err != nil {
		t.Errorf("après clôture, une nouvelle entrée doit passer: %v", err)
	}
}

func TestCircuitBreakerTriggersAndCoolsDown(t *testing.T) {
	m, clock := newTestManager(testConfig())
	ctx := context.Background()

	// 3 pertes de 20 : nette -60, seuil 5 % de ~940 = 47 → déclenchement.
	for i := 0; i < 3; i++ {
		if err := m.RecordTradeResult(ctx, types.TradeResult{PnL: -20}); err != nil {
			t.Fatal(err)
		}
	}

	active, err := m.CircuitBreakerActive(ctx)
	if err != nil || !active {
		t.Fatalf("breaker doit être actif (active=%v, err=%v)", active, err)
	}
	if err := m.ValidateEntry(ctx, validSignal()); !errors.Is(err, ErrBreakerActive) {
		t.Errorf("entrée refusée pendant la suspension, obtenu %v", err)
	}

	// Avant la fin du cooldown : toujours actif.
	*clock = clock.Add(2 * time.Hour)
	if active, _ := m.CircuitBreakerActive(ctx); !active {
		t.Fatal("breaker encore actif à mi-cooldown")
	}

	// Après le cooldown : levé, et la fenêtre est purgée.
	*clock = clock.Add(3 * time.Hour)
	if active, _ := m.CircuitBreakerActive(ctx); active {
		t.Fatal("breaker doit être levé après le cooldown")
	}
	if err := m.ValidateEntry(ctx, validSignal()); err != nil {
		t.Errorf("après la levée, une entrée doit passer: %v", err)
	}
}

func TestCircuitBreakerWindowSlides(t *testing.T) {
	cfg := testConfig()
	cfg.BreakerLookback = 3
	m, _ := newTestManager(cfg)
	ctx := context.Background()

	// Une grosse perte ancienne suivie de gains : la fenêtre glissante
	// (3 trades) ne voit plus la perte → pas de déclenchement.
	m.RecordTradeResult(ctx, types.TradeResult{PnL: -100})
	// -100 sur capital 900 : seuil 45 → breaker déclenché immédiatement.
	if active, _ := m.CircuitBreakerActive(ctx); !active {
		t.Fatal("le breaker doit se déclencher sur la grosse perte")
	}
	m.ResetBreaker()

	m.RecordTradeResult(ctx, types.TradeResult{PnL: 5})
	m.RecordTradeResult(ctx, types.TradeResult{PnL: 5})
	m.RecordTradeResult(ctx, types.TradeResult{PnL: -10})
	// Fenêtre = [5, 5, -10] → nette 0 : pas de déclenchement.
	if active, _ := m.CircuitBreakerActive(ctx); active {
		t.Error("fenêtre glissante nette nulle: pas de suspension")
	}
}

func TestEventSinkReceivesBreakerEvent(t *testing.T) {
	m, _ := newTestManager(testConfig())
	events := make(chan types.RiskEvent, 4)
	m.SetEventSink(func(ev types.RiskEvent) { events <- ev })

	for i := 0; i < 3; i++ {
		m.RecordTradeResult(context.Background(), types.TradeResult{PnL: -20})
	}

	select {
	case ev := <-events:
		if ev.Kind != types.RiskEventCircuitBreaker {
			t.Errorf("kind = %s, attendu CIRCUIT_BREAKER", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("aucun événement de risque reçu")
	}
}

func TestExitFor(t *testing.T) {
	m, _ := newTestManager(testConfig())
	sl, tp := m.ExitFor(60000)
	if !almostEqual(sl, 59400) || !almostEqual(tp, 61200) {
		t.Errorf("ExitFor = %v/%v, attendu 59400/61200", sl, tp)
	}
}
