package ordermanager

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// fakeClient simule le client REST Binance.
type fakeClient struct {
	filters   types.SymbolFilters
	open      []types.Order
	balances  []types.Balance
	orders    map[int64]types.Order // état interrogeable par QueryOrder
	nextID    int64
	placed    []types.OrderRequest
	canceled  []int64
	placeHook func(req types.OrderRequest) (types.Order, error)
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		filters: types.SymbolFilters{
			Symbol: "BTCUSDT", StepSize: 0.00001, TickSize: 0.01,
			MinQty: 0.00001, MaxQty: 9000, MinNotional: 5,
			QtyDecimals: 5, PriceDecimals: 2,
		},
		orders: map[int64]types.Order{},
		nextID: 1000,
	}
}

func (f *fakeClient) PlaceOrder(_ context.Context, req types.OrderRequest, _ types.SymbolFilters) (types.Order, error) {
	if f.placeHook != nil {
		return f.placeHook(req)
	}
	f.placed = append(f.placed, req)
	f.nextID++
	o := types.Order{
		OrderID:       f.nextID,
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Type:          req.Type,
		TimeInForce:   req.TimeInForce,
		Price:         req.Price,
		OrigQuantity:  req.Quantity,
		Status:        types.OrderStatusNew,
	}
	f.orders[o.OrderID] = o
	return o, nil
}

func (f *fakeClient) CancelOrder(_ context.Context, _ string, orderID int64) error {
	f.canceled = append(f.canceled, orderID)
	if o, ok := f.orders[orderID]; ok {
		o.Status = types.OrderStatusCanceled
		f.orders[orderID] = o
	}
	return nil
}

func (f *fakeClient) QueryOrder(_ context.Context, _ string, orderID int64) (types.Order, error) {
	o, ok := f.orders[orderID]
	if !ok {
		return types.Order{}, errors.New("ordre inconnu")
	}
	return o, nil
}

func (f *fakeClient) OpenOrders(context.Context, string) ([]types.Order, error) {
	return f.open, nil
}

func (f *fakeClient) Account(context.Context) ([]types.Balance, error) {
	return f.balances, nil
}

func (f *fakeClient) ExchangeFilters(context.Context, string) (types.SymbolFilters, error) {
	return f.filters, nil
}

func newTestManager(t *testing.T, fc *fakeClient) *Manager {
	t.Helper()
	m := New(fc, "BTCUSDT", 0.0001, slog.New(slog.DiscardHandler))
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return m
}

func TestPlaceOrderRequiresReconcile(t *testing.T) {
	m := New(newFakeClient(), "BTCUSDT", 0.0001, slog.New(slog.DiscardHandler))
	_, err := m.PlaceOrder(context.Background(), types.OrderRequest{Type: types.OrderTypeLimit})
	if !errors.Is(err, ErrNotReconciled) {
		t.Fatalf("attendu ErrNotReconciled, obtenu %v", err)
	}
}

func TestPlaceOrderEnforcesLimitGTC(t *testing.T) {
	m := newTestManager(t, newFakeClient())

	if _, err := m.PlaceOrder(context.Background(), types.OrderRequest{
		Type: types.OrderTypeMarket, Price: 60000, Quantity: 0.001,
	}); err == nil || !strings.Contains(err.Error(), "LIMIT") {
		t.Errorf("un ordre non LIMIT doit être refusé: %v", err)
	}

	if _, err := m.PlaceOrder(context.Background(), types.OrderRequest{
		Type: types.OrderTypeLimit, TimeInForce: types.TimeInForceIOC, Price: 60000, Quantity: 0.001,
	}); err == nil || !strings.Contains(err.Error(), "GTC") {
		t.Errorf("un ordre non GTC doit être refusé: %v", err)
	}
}

func TestPlaceOrderQuantizesAndGeneratesClientID(t *testing.T) {
	fc := newFakeClient()
	m := newTestManager(t, fc)

	order, err := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit,
		Price: 60123.456789, Quantity: 0.001234567,
	})
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if len(fc.placed) != 1 {
		t.Fatalf("1 ordre attendu, %d placés", len(fc.placed))
	}
	sent := fc.placed[0]
	if sent.Price != 60123.45 || sent.Quantity != 0.00123 {
		t.Errorf("quantification non appliquée: %+v", sent)
	}
	if !strings.HasPrefix(sent.ClientOrderID, clientIDPrefix) {
		t.Errorf("ClientOrderID sans préfixe bot: %q", sent.ClientOrderID)
	}
	if sent.TimeInForce != types.TimeInForceGTC {
		t.Errorf("TimeInForce = %s, attendu GTC", sent.TimeInForce)
	}
	if order.OrderID == 0 {
		t.Error("ordre retourné sans ID")
	}
}

func TestPlaceOrderPreventsDoubleOrder(t *testing.T) {
	fc := newFakeClient()
	m := newTestManager(t, fc)

	if _, err := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.001,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.001,
	})
	if !errors.Is(err, ErrOrderExists) {
		t.Fatalf("le double ordre doit être refusé, obtenu %v", err)
	}
}

func TestPlaceOrderChecksExchangeBeforeCreation(t *testing.T) {
	fc := newFakeClient()
	// Un ordre du bot existe côté exchange (ex: après redémarrage d'un
	// autre process) mais pas dans l'état local.
	fc.open = []types.Order{{
		OrderID: 7, ClientOrderID: clientIDPrefix + "ghost",
		Status: types.OrderStatusNew,
	}}
	m := New(fc, "BTCUSDT", 0.0001, slog.New(slog.DiscardHandler))
	// Réconciliation sur un état vide, puis apparition de l'ordre fantôme.
	saved := fc.open
	fc.open = nil
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	fc.open = saved

	_, err := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.001,
	})
	if !errors.Is(err, ErrOrderExists) {
		t.Fatalf("la vérification d'existence pré-création doit refuser: %v", err)
	}
}

func TestReconcileAdoptsBotOrders(t *testing.T) {
	fc := newFakeClient()
	fc.open = []types.Order{
		{OrderID: 1, ClientOrderID: "manual-x", Status: types.OrderStatusNew},
		{OrderID: 2, ClientOrderID: clientIDPrefix + "abc", Status: types.OrderStatusPartiallyFilled,
			Price: 60000, OrigQuantity: 0.002, ExecutedQuantity: 0.0005, CumQuote: 30},
	}
	m := newTestManager(t, fc)

	entry, ok := m.EntryOrder()
	if !ok || entry.OrderID != 2 {
		t.Fatalf("l'ordre du bot doit être adopté: %+v (ok=%v)", entry, ok)
	}
	pos := m.Position()
	if pos.Quantity != 0.0005 || pos.AvgEntry != 60000 {
		t.Errorf("position issue du fill partiel adopté incorrecte: %+v", pos)
	}
}

func TestReplaceOrderFilledNoReplace(t *testing.T) {
	fc := newFakeClient()
	m := newTestManager(t, fc)

	placed, _ := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.001,
	})

	// L'ordre est exécuté intégralement avant le remplacement.
	o := fc.orders[placed.OrderID]
	o.Status = types.OrderStatusFilled
	o.ExecutedQuantity = 0.001
	o.CumQuote = 60
	fc.orders[placed.OrderID] = o

	got, err := m.ReplaceOrder(context.Background(), "BTCUSDT", placed.OrderID, types.OrderRequest{Price: 59990})
	if err != nil {
		t.Fatalf("ReplaceOrder: %v", err)
	}
	if got.Status != types.OrderStatusFilled {
		t.Errorf("statut = %s, attendu FILLED", got.Status)
	}
	if len(fc.canceled) != 0 {
		t.Error("un ordre FILLED ne doit pas être annulé")
	}
	pos := m.Position()
	if pos.Quantity != 0.001 || pos.AvgEntry != 60000 {
		t.Errorf("position après fill: %+v", pos)
	}
}

func TestReplaceOrderPartialFillCancelsAndReplacesRemainder(t *testing.T) {
	fc := newFakeClient()
	m := newTestManager(t, fc)

	placed, _ := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.002,
	})

	// Fill partiel : 0.0005 exécuté à 60000.
	o := fc.orders[placed.OrderID]
	o.Status = types.OrderStatusPartiallyFilled
	o.ExecutedQuantity = 0.0005
	o.CumQuote = 30
	fc.orders[placed.OrderID] = o

	newOrder, err := m.ReplaceOrder(context.Background(), "BTCUSDT", placed.OrderID,
		types.OrderRequest{Side: types.SideBuy, Price: 60010})
	if err != nil {
		t.Fatalf("ReplaceOrder: %v", err)
	}

	if len(fc.canceled) != 1 || fc.canceled[0] != placed.OrderID {
		t.Errorf("l'ancien ordre doit être annulé: %v", fc.canceled)
	}
	// Reliquat : 0.002 - 0.0005 = 0.0015 replacé au nouveau prix.
	if newOrder.OrigQuantity != 0.0015 || newOrder.Price != 60010 {
		t.Errorf("reliquat replacé incorrect: %+v", newOrder)
	}
	// La position porte le fill partiel au prix moyen exact.
	pos := m.Position()
	if pos.Quantity != 0.0005 || pos.AvgEntry != 60000 {
		t.Errorf("position après fill partiel: %+v", pos)
	}
}

func TestReplaceOrderDustRemainder(t *testing.T) {
	fc := newFakeClient()
	m := newTestManager(t, fc)

	placed, _ := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.001,
	})

	// Quasi tout exécuté : reliquat 0.00001 → notionnel < minNotional (5).
	o := fc.orders[placed.OrderID]
	o.Status = types.OrderStatusPartiallyFilled
	o.ExecutedQuantity = 0.00099
	o.CumQuote = 59.4
	fc.orders[placed.OrderID] = o

	_, err := m.ReplaceOrder(context.Background(), "BTCUSDT", placed.OrderID,
		types.OrderRequest{Side: types.SideBuy, Price: 60010})
	if !errors.Is(err, ErrRemainderDust) {
		t.Fatalf("attendu ErrRemainderDust, obtenu %v", err)
	}
	if len(fc.canceled) != 1 {
		t.Error("le reliquat poussière doit malgré tout être annulé")
	}
}

func TestShouldReplace(t *testing.T) {
	m := New(newFakeClient(), "BTCUSDT", 0.0001, slog.New(slog.DiscardHandler)) // seuil 0,01 %

	if m.ShouldReplace(60000, 60003) { // 0,005 % : sous le seuil
		t.Error("dérive de 0,005 % : pas de remplacement")
	}
	if !m.ShouldReplace(60000, 60012) { // 0,02 % : au-dessus
		t.Error("dérive de 0,02 % : remplacement requis")
	}
	if !m.ShouldReplace(60000, 59985) { // dérive à la baisse aussi
		t.Error("la dérive à la baisse doit aussi déclencher")
	}
	if m.ShouldReplace(0, 60000) {
		t.Error("prix d'ordre nul : jamais de remplacement")
	}
}

func TestPositionAvgEntryAcrossReplacements(t *testing.T) {
	// Fills à des prix différents via remplacements successifs : le prix
	// moyen doit être exact (pondéré par cummulativeQuoteQty).
	fc := newFakeClient()
	m := newTestManager(t, fc)

	placed, _ := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.002,
	})

	o := fc.orders[placed.OrderID]
	o.Status = types.OrderStatusPartiallyFilled
	o.ExecutedQuantity = 0.001
	o.CumQuote = 60 // fill à 60000
	fc.orders[placed.OrderID] = o

	second, err := m.ReplaceOrder(context.Background(), "BTCUSDT", placed.OrderID,
		types.OrderRequest{Side: types.SideBuy, Price: 60200})
	if err != nil {
		t.Fatal(err)
	}

	// Le nouvel ordre est ensuite totalement exécuté à 60200.
	o = fc.orders[second.OrderID]
	o.Status = types.OrderStatusFilled
	o.ExecutedQuantity = 0.001
	o.CumQuote = 60.2
	fc.orders[second.OrderID] = o

	if _, err := m.ReplaceOrder(context.Background(), "BTCUSDT", second.OrderID,
		types.OrderRequest{Side: types.SideBuy, Price: 60300}); err != nil {
		t.Fatal(err)
	}

	pos := m.Position()
	if pos.Quantity != 0.002 {
		t.Fatalf("quantité = %v, attendu 0.002", pos.Quantity)
	}
	wantAvg := (60.0 + 60.2) / 0.002
	if diff := pos.AvgEntry - wantAvg; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("prix moyen = %v, attendu %v", pos.AvgEntry, wantAvg)
	}
}

func TestExitCalculatorApplied(t *testing.T) {
	fc := newFakeClient()
	m := newTestManager(t, fc)
	m.SetExitCalculator(func(avg float64) (float64, float64) {
		return avg * 0.99, avg * 1.02
	})

	placed, _ := m.PlaceOrder(context.Background(), types.OrderRequest{
		Side: types.SideBuy, Type: types.OrderTypeLimit, Price: 60000, Quantity: 0.001,
	})
	o := fc.orders[placed.OrderID]
	o.Status = types.OrderStatusFilled
	o.ExecutedQuantity = 0.001
	o.CumQuote = 60
	fc.orders[placed.OrderID] = o
	if _, err := m.ReplaceOrder(context.Background(), "BTCUSDT", placed.OrderID, types.OrderRequest{}); err != nil {
		t.Fatal(err)
	}

	pos := m.Position()
	if pos.StopLoss != 60000*0.99 || pos.TakeProfit != 60000*1.02 {
		t.Errorf("SL/TP non recalculés: %+v", pos)
	}
}
