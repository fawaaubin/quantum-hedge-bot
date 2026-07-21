package gateway

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// combinedMessage est l'enveloppe des flux combinés Binance
// (/stream?streams=...).
type combinedMessage struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

// eventHeader permet de discriminer le type d'événement. Le champ E est
// déclaré pour éviter que la clé JSON "E" (nombre) ne soit associée par
// correspondance insensible à la casse au champ "e" (chaîne).
type eventHeader struct {
	EventType string `json:"e"`
	EventTime int64  `json:"E"`
}

// wsTrade est l'événement brut du flux <symbol>@trade.
type wsTrade struct {
	EventType    string `json:"e"`
	EventTime    int64  `json:"E"`
	Symbol       string `json:"s"`
	TradeID      int64  `json:"t"`
	Price        string `json:"p"`
	Quantity     string `json:"q"`
	TradeTime    int64  `json:"T"`
	IsBuyerMaker bool   `json:"m"`
}

// wsDepth est l'événement brut du flux <symbol>@depth (diff stream).
type wsDepth struct {
	EventType     string      `json:"e"`
	EventTime     int64       `json:"E"`
	Symbol        string      `json:"s"`
	FirstUpdateID int64       `json:"U"`
	FinalUpdateID int64       `json:"u"`
	Bids          [][2]string `json:"b"`
	Asks          [][2]string `json:"a"`
}

// parsedEvent est le résultat normalisé du parsing d'un message.
// Exactement un des deux champs est non nil.
type parsedEvent struct {
	trade *types.Tick
	depth *types.DepthUpdate
}

// parseCombined décode un message du flux combiné vers un événement
// normalisé. Les types d'événements inconnus retournent une erreur.
func parseCombined(raw []byte) (parsedEvent, error) {
	var msg combinedMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return parsedEvent{}, fmt.Errorf("enveloppe stream: %w", err)
	}
	data := msg.Data
	if data == nil {
		// Message hors flux combiné (ex: flux brut /ws/...).
		data = raw
	}

	var head eventHeader
	if err := json.Unmarshal(data, &head); err != nil {
		return parsedEvent{}, fmt.Errorf("entête événement: %w", err)
	}

	switch head.EventType {
	case "trade":
		var t wsTrade
		if err := json.Unmarshal(data, &t); err != nil {
			return parsedEvent{}, fmt.Errorf("parse trade: %w", err)
		}
		price, err := strconv.ParseFloat(t.Price, 64)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("prix trade %q: %w", t.Price, err)
		}
		qty, err := strconv.ParseFloat(t.Quantity, 64)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("quantité trade %q: %w", t.Quantity, err)
		}
		return parsedEvent{trade: &types.Tick{
			Symbol:    t.Symbol,
			Timestamp: time.UnixMilli(t.TradeTime).UTC(),
			Price:     price,
			Quantity:  qty,
		}}, nil

	case "depthUpdate":
		var d wsDepth
		if err := json.Unmarshal(data, &d); err != nil {
			return parsedEvent{}, fmt.Errorf("parse depth: %w", err)
		}
		bids, err := parseLevels(d.Bids)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("bids: %w", err)
		}
		asks, err := parseLevels(d.Asks)
		if err != nil {
			return parsedEvent{}, fmt.Errorf("asks: %w", err)
		}
		return parsedEvent{depth: &types.DepthUpdate{
			Symbol:        d.Symbol,
			FirstUpdateID: d.FirstUpdateID,
			FinalUpdateID: d.FinalUpdateID,
			Bids:          bids,
			Asks:          asks,
			EventTime:     time.UnixMilli(d.EventTime).UTC(),
		}}, nil

	default:
		return parsedEvent{}, fmt.Errorf("type d'événement inconnu: %q", head.EventType)
	}
}

// parseLevels convertit les niveaux [prix, quantité] de Binance.
// Les quantités nulles sont conservées : elles signifient "supprimer
// le niveau" dans les updates incrémentales.
func parseLevels(in [][2]string) ([]types.PriceLevel, error) {
	out := make([]types.PriceLevel, 0, len(in))
	for _, pair := range in {
		price, err := strconv.ParseFloat(pair[0], 64)
		if err != nil {
			return nil, fmt.Errorf("prix %q: %w", pair[0], err)
		}
		qty, err := strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return nil, fmt.Errorf("quantité %q: %w", pair[1], err)
		}
		out = append(out, types.PriceLevel{Price: price, Quantity: qty})
	}
	return out, nil
}
