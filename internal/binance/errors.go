package binance

import (
	"errors"
	"fmt"
)

// APIError est une erreur structurée retournée par l'API Binance.
type APIError struct {
	Status int    // code HTTP
	Code   int    // code d'erreur Binance (ex: -2011)
	Msg    string // message Binance
}

func (e *APIError) Error() string {
	return fmt.Sprintf("binance HTTP %d, code %d: %s", e.Status, e.Code, e.Msg)
}

// Codes d'erreur Binance utiles à la logique d'ordres.
const (
	// CodeUnknownOrder : l'ordre n'existe pas (déjà exécuté ou annulé).
	CodeUnknownOrder = -2011
)

// IsRateLimited indique une réponse 429 (trop de requêtes) ou
// 418 (bannissement IP temporaire) : le client a déjà appliqué la
// pénalité de délai adaptatif, l'appelant doit espacer ses tentatives.
func IsRateLimited(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == 429 || apiErr.Status == 418
	}
	return false
}

// IsUnknownOrder indique que l'ordre visé n'existe plus côté exchange.
func IsUnknownOrder(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == CodeUnknownOrder
	}
	return false
}
