package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// orderResponse est la réponse brute des endpoints d'ordre
// (newOrderRespType=RESULT pour POST, format identique en GET/DELETE).
type orderResponse struct {
	Symbol          string `json:"symbol"`
	OrderID         int64  `json:"orderId"`
	ClientOrderID   string `json:"clientOrderId"`
	OrigClientOrder string `json:"origClientOrderId"`
	TransactTime    int64  `json:"transactTime"`
	Time            int64  `json:"time"`
	UpdateTime      int64  `json:"updateTime"`
	Price           string `json:"price"`
	OrigQty         string `json:"origQty"`
	ExecutedQty     string `json:"executedQty"`
	CumulativeQuote string `json:"cummulativeQuoteQty"`
	Status          string `json:"status"`
	TimeInForce     string `json:"timeInForce"`
	Type            string `json:"type"`
	Side            string `json:"side"`
}

// toOrder convertit la réponse brute en types.Order.
func (r orderResponse) toOrder() (types.Order, error) {
	parse := func(name, s string) (float64, error) {
		if s == "" {
			return 0, nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("%s %q: %w", name, s, err)
		}
		return v, nil
	}
	price, err := parse("price", r.Price)
	if err != nil {
		return types.Order{}, err
	}
	origQty, err := parse("origQty", r.OrigQty)
	if err != nil {
		return types.Order{}, err
	}
	execQty, err := parse("executedQty", r.ExecutedQty)
	if err != nil {
		return types.Order{}, err
	}
	cumQuote, err := parse("cummulativeQuoteQty", r.CumulativeQuote)
	if err != nil {
		return types.Order{}, err
	}

	clientID := r.ClientOrderID
	if clientID == "" {
		clientID = r.OrigClientOrder
	}
	created := r.Time
	if created == 0 {
		created = r.TransactTime
	}
	updated := r.UpdateTime
	if updated == 0 {
		updated = r.TransactTime
	}

	return types.Order{
		OrderID:          r.OrderID,
		ClientOrderID:    clientID,
		Symbol:           r.Symbol,
		Side:             types.Side(r.Side),
		Type:             types.OrderType(r.Type),
		TimeInForce:      types.TimeInForce(r.TimeInForce),
		Price:            price,
		OrigQuantity:     origQty,
		ExecutedQuantity: execQty,
		CumQuote:         cumQuote,
		Status:           types.OrderStatus(r.Status),
		CreatedAt:        time.UnixMilli(created).UTC(),
		UpdatedAt:        time.UnixMilli(updated).UTC(),
	}, nil
}

// PlaceOrder envoie un ordre signé (POST /api/v3/order). Les quantités
// et prix doivent déjà être quantifiés selon les filtres du symbole.
func (c *Client) PlaceOrder(ctx context.Context, req types.OrderRequest, f types.SymbolFilters) (types.Order, error) {
	params := url.Values{}
	params.Set("symbol", req.Symbol)
	params.Set("side", string(req.Side))
	params.Set("type", string(req.Type))
	params.Set("quantity", FormatQty(f, req.Quantity))
	if req.Type == types.OrderTypeLimit {
		params.Set("timeInForce", string(req.TimeInForce))
		params.Set("price", FormatPrice(f, req.Price))
	}
	if req.ClientOrderID != "" {
		params.Set("newClientOrderId", req.ClientOrderID)
	}
	params.Set("newOrderRespType", "RESULT")

	var raw orderResponse
	if err := c.doSigned(ctx, http.MethodPost, "/api/v3/order", params, &raw); err != nil {
		return types.Order{}, err
	}
	return raw.toOrder()
}

// CancelOrder annule un ordre (DELETE /api/v3/order).
func (c *Client) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("orderId", strconv.FormatInt(orderID, 10))
	return c.doSigned(ctx, http.MethodDelete, "/api/v3/order", params, nil)
}

// QueryOrder interroge l'état d'un ordre (GET /api/v3/order) — requis
// avant tout remplacement.
func (c *Client) QueryOrder(ctx context.Context, symbol string, orderID int64) (types.Order, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("orderId", strconv.FormatInt(orderID, 10))

	var raw orderResponse
	if err := c.doSigned(ctx, http.MethodGet, "/api/v3/order", params, &raw); err != nil {
		return types.Order{}, err
	}
	return raw.toOrder()
}

// OpenOrders liste les ordres ouverts du symbole (GET /api/v3/openOrders).
func (c *Client) OpenOrders(ctx context.Context, symbol string) ([]types.Order, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	var raws []orderResponse
	if err := c.doSigned(ctx, http.MethodGet, "/api/v3/openOrders", params, &raws); err != nil {
		return nil, err
	}
	orders := make([]types.Order, 0, len(raws))
	for _, r := range raws {
		o, err := r.toOrder()
		if err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, nil
}

// Account retourne les soldes non nuls du compte (GET /api/v3/account).
func (c *Client) Account(ctx context.Context) ([]types.Balance, error) {
	var raw struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   string `json:"free"`
			Locked string `json:"locked"`
		} `json:"balances"`
	}
	if err := c.doSigned(ctx, http.MethodGet, "/api/v3/account", nil, &raw); err != nil {
		return nil, err
	}

	balances := make([]types.Balance, 0, len(raw.Balances))
	for _, b := range raw.Balances {
		free, err := strconv.ParseFloat(b.Free, 64)
		if err != nil {
			return nil, fmt.Errorf("solde free %s: %w", b.Asset, err)
		}
		locked, err := strconv.ParseFloat(b.Locked, 64)
		if err != nil {
			return nil, fmt.Errorf("solde locked %s: %w", b.Asset, err)
		}
		if free > 0 || locked > 0 {
			balances = append(balances, types.Balance{Asset: b.Asset, Free: free, Locked: locked})
		}
	}
	return balances, nil
}
