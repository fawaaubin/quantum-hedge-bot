package binance

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// ExchangeFilters récupère et parse les filtres du symbole depuis
// GET /api/v3/exchangeInfo (endpoint public).
func (c *Client) ExchangeFilters(ctx context.Context, symbol string) (types.SymbolFilters, error) {
	params := url.Values{}
	params.Set("symbol", symbol)

	var raw struct {
		Symbols []struct {
			Symbol  string `json:"symbol"`
			Filters []struct {
				FilterType  string `json:"filterType"`
				StepSize    string `json:"stepSize"`
				TickSize    string `json:"tickSize"`
				MinQty      string `json:"minQty"`
				MaxQty      string `json:"maxQty"`
				MinNotional string `json:"minNotional"`
			} `json:"filters"`
		} `json:"symbols"`
	}
	if err := c.doPublic(ctx, http.MethodGet, "/api/v3/exchangeInfo", params, &raw); err != nil {
		return types.SymbolFilters{}, err
	}
	if len(raw.Symbols) == 0 {
		return types.SymbolFilters{}, fmt.Errorf("symbole %s absent d'exchangeInfo", symbol)
	}

	f := types.SymbolFilters{Symbol: raw.Symbols[0].Symbol}
	parse := func(name, s string) (float64, error) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("filtre %s %q: %w", name, s, err)
		}
		return v, nil
	}

	for _, flt := range raw.Symbols[0].Filters {
		var err error
		switch flt.FilterType {
		case "LOT_SIZE":
			if f.StepSize, err = parse("stepSize", flt.StepSize); err != nil {
				return types.SymbolFilters{}, err
			}
			if f.MinQty, err = parse("minQty", flt.MinQty); err != nil {
				return types.SymbolFilters{}, err
			}
			if f.MaxQty, err = parse("maxQty", flt.MaxQty); err != nil {
				return types.SymbolFilters{}, err
			}
			f.QtyDecimals = decimalsOf(flt.StepSize)
		case "PRICE_FILTER":
			if f.TickSize, err = parse("tickSize", flt.TickSize); err != nil {
				return types.SymbolFilters{}, err
			}
			f.PriceDecimals = decimalsOf(flt.TickSize)
		case "NOTIONAL", "MIN_NOTIONAL":
			if flt.MinNotional != "" {
				if f.MinNotional, err = parse("minNotional", flt.MinNotional); err != nil {
					return types.SymbolFilters{}, err
				}
			}
		}
	}

	if f.StepSize <= 0 || f.TickSize <= 0 {
		return types.SymbolFilters{}, fmt.Errorf("filtres incomplets pour %s: %+v", symbol, f)
	}
	return f, nil
}

// ValidateAndQuantize quantifie la quantité (stepSize, plancher) et le
// prix (tickSize, plancher) puis vérifie minQty, maxQty et minNotional.
// Retourne la requête ajustée, prête à être envoyée.
func ValidateAndQuantize(req types.OrderRequest, f types.SymbolFilters) (types.OrderRequest, error) {
	if f.StepSize <= 0 || f.TickSize <= 0 {
		return req, fmt.Errorf("filtres non chargés pour %s", req.Symbol)
	}

	req.Quantity = floorToStep(req.Quantity, f.StepSize, f.QtyDecimals)
	req.Price = floorToStep(req.Price, f.TickSize, f.PriceDecimals)

	if req.Quantity < f.MinQty {
		return req, fmt.Errorf("quantité %s < minQty %s",
			FormatQty(f, req.Quantity), FormatQty(f, f.MinQty))
	}
	if f.MaxQty > 0 && req.Quantity > f.MaxQty {
		return req, fmt.Errorf("quantité %s > maxQty %s",
			FormatQty(f, req.Quantity), FormatQty(f, f.MaxQty))
	}
	if req.Type == types.OrderTypeLimit && req.Price <= 0 {
		return req, fmt.Errorf("prix invalide pour un ordre LIMIT: %v", req.Price)
	}
	if notional := req.Price * req.Quantity; f.MinNotional > 0 && notional < f.MinNotional {
		return req, fmt.Errorf("notionnel %.8f < minNotional %.8f", notional, f.MinNotional)
	}
	return req, nil
}

// FormatQty formate une quantité avec la précision du stepSize.
func FormatQty(f types.SymbolFilters, q float64) string {
	return strconv.FormatFloat(q, 'f', f.QtyDecimals, 64)
}

// FormatPrice formate un prix avec la précision du tickSize.
func FormatPrice(f types.SymbolFilters, p float64) string {
	return strconv.FormatFloat(p, 'f', f.PriceDecimals, 64)
}

// floorToStep arrondit v au multiple inférieur de step, puis élimine le
// bruit flottant en arrondissant à la précision décimale du filtre.
func floorToStep(v, step float64, decimals int) float64 {
	if step <= 0 {
		return v
	}
	floored := math.Floor(v/step+1e-9) * step
	scale := math.Pow10(decimals)
	return math.Round(floored*scale) / scale
}

// decimalsOf retourne le nombre de décimales significatives d'une taille
// de pas exprimée en chaîne (ex: "0.00001000" → 5, "1.00000000" → 0).
func decimalsOf(step string) int {
	i := strings.IndexByte(step, '.')
	if i < 0 {
		return 0
	}
	frac := strings.TrimRight(step[i+1:], "0")
	return len(frac)
}
