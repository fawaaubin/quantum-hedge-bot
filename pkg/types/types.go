// Package types contient les types de domaine partagés entre tous les
// modules du bot. Aucune logique Binance ni logique métier ne doit vivre ici :
// uniquement des structures de données et des constantes.
package types

import (
	"errors"
	"time"
)

// ErrNotImplemented est retourné par les squelettes de la Phase 1.
// Chaque phase suivante remplace ces retours par une implémentation réelle.
var ErrNotImplemented = errors.New("not implemented (phase ultérieure)")

// ShutdownTimeout borne la durée de l'arrêt propre (drainage des
// goroutines, sauvegarde finale de l'état).
const ShutdownTimeout = 10 * time.Second

// Side représente le sens d'un ordre ou d'une position.
type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

// OrderType représente le type d'ordre Binance supporté.
type OrderType string

const (
	OrderTypeLimit  OrderType = "LIMIT"
	OrderTypeMarket OrderType = "MARKET"
)

// TimeInForce représente la durée de validité d'un ordre.
type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "GTC"
	TimeInForceIOC TimeInForce = "IOC"
	TimeInForceFOK TimeInForce = "FOK"
)

// OrderStatus représente l'état d'un ordre côté exchange.
type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "NEW"
	OrderStatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	OrderStatusFilled          OrderStatus = "FILLED"
	OrderStatusCanceled        OrderStatus = "CANCELED"
	OrderStatusRejected        OrderStatus = "REJECTED"
	OrderStatusExpired         OrderStatus = "EXPIRED"
)

// EngineState est l'état de la machine à états du moteur de décision.
type EngineState string

const (
	StateIdle         EngineState = "IDLE"
	StateWaitingEntry EngineState = "WAITING_ENTRY"
	StateInPosition   EngineState = "IN_POSITION"
	StateWaitingExit  EngineState = "WAITING_EXIT"
)

// MarketEventKind distingue les événements publiés par la gateway.
type MarketEventKind string

const (
	EventTrade MarketEventKind = "TRADE"
	EventDepth MarketEventKind = "DEPTH"
)

// Tick représente une transaction (trade) observée sur le marché,
// enrichie du meilleur bid/ask au moment de l'observation.
// C'est aussi l'unité de rejeu du module de backtest.
type Tick struct {
	Symbol       string
	Timestamp    time.Time
	Price        float64
	Quantity     float64
	BestBid      float64
	BestAsk      float64
	BidVolume    float64
	AskVolume    float64
	LastUpdateID int64
}

// PriceLevel est un niveau de prix du carnet d'ordres.
type PriceLevel struct {
	Price    float64
	Quantity float64
}

// DepthUpdate est une mise à jour incrémentale du carnet d'ordres.
type DepthUpdate struct {
	Symbol        string
	FirstUpdateID int64 // champ U de Binance
	FinalUpdateID int64 // champ u de Binance
	Bids          []PriceLevel
	Asks          []PriceLevel
	EventTime     time.Time
}

// OrderBook est l'état local reconstruit du carnet d'ordres.
type OrderBook struct {
	Symbol       string
	LastUpdateID int64
	Bids         []PriceLevel // triés par prix décroissant
	Asks         []PriceLevel // triés par prix croissant
	UpdatedAt    time.Time
}

// MarketEvent est l'enveloppe publiée par la gateway sur son channel.
// Exactement un des champs Tick/Depth est renseigné selon Kind.
type MarketEvent struct {
	Kind  MarketEventKind
	Tick  *Tick
	Depth *DepthUpdate
}

// OrderRequest décrit une demande de placement d'ordre, indépendante
// du transport Binance.
type OrderRequest struct {
	Symbol        string
	Side          Side
	Type          OrderType
	TimeInForce   TimeInForce
	Price         float64
	Quantity      float64
	ClientOrderID string // pour l'idempotence
}

// Order représente un ordre connu du bot (local ou confirmé par l'exchange).
type Order struct {
	OrderID          int64
	ClientOrderID    string
	Symbol           string
	Side             Side
	Type             OrderType
	TimeInForce      TimeInForce
	Price            float64
	OrigQuantity     float64
	ExecutedQuantity float64
	// CumQuote est le montant quote cumulé réellement exécuté
	// (cummulativeQuoteQty) : sert au calcul exact du prix moyen.
	CumQuote  float64
	Status    OrderStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsOpen indique si l'ordre est encore actif côté exchange.
func (o Order) IsOpen() bool {
	return o.Status == OrderStatusNew || o.Status == OrderStatusPartiallyFilled
}

// Position représente une position ouverte ou clôturée.
type Position struct {
	ID           int64
	Symbol       string
	Side         Side
	Quantity     float64
	AvgEntry     float64
	StopLoss     float64
	TakeProfit   float64
	TrailingStop float64
	OpenedAt     time.Time
	ClosedAt     *time.Time
	RealizedPnL  float64
}

// TradeResult est le résultat d'un trade clôturé, consommé par le risk
// manager (circuit breaker) et persisté par le store.
type TradeResult struct {
	Symbol    string
	Side      Side
	Quantity  float64
	Entry     float64
	Exit      float64
	PnL       float64
	ClosedAt  time.Time
	Slippage  float64
	ReasonTag string // ex: "stop_loss", "take_profit", "signal_exit"
}

// SignalAction indique l'action recommandée par le moteur de décision.
type SignalAction string

const (
	ActionEnterLong SignalAction = "ENTER_LONG"
	ActionExit      SignalAction = "EXIT"
	ActionNone      SignalAction = "NONE"
)

// Signal est la sortie du moteur de décision, consommée par l'order manager
// après validation par le risk manager.
type Signal struct {
	Symbol    string
	Action    SignalAction
	Price     float64
	Timestamp time.Time
	Reason    string
}

// ExitLevels regroupe les niveaux de sortie calculés par le risk manager.
type ExitLevels struct {
	StopLoss     float64
	TakeProfit   float64
	TrailingStop float64
}

// SymbolFilters porte les filtres Binance nécessaires à la validation
// locale des ordres (Phase 3).
type SymbolFilters struct {
	Symbol      string
	StepSize    float64
	TickSize    float64
	MinQty      float64
	MaxQty      float64
	MinNotional float64
	// Précisions décimales dérivées de stepSize/tickSize, pour formater
	// les quantités et prix envoyés à l'API sans bruit flottant.
	QtyDecimals   int
	PriceDecimals int
}

// Balance est le solde d'un actif du compte.
type Balance struct {
	Asset  string
	Free   float64
	Locked float64
}

// BotState est l'état global persisté du bot, rechargé au démarrage.
type BotState struct {
	EngineState       EngineState
	CircuitBreakerOn  bool
	Capital           float64
	OpenPositionCount int
	UpdatedAt         time.Time
}

// RiskEventKind catégorise les événements de risque persistés.
type RiskEventKind string

const (
	RiskEventCircuitBreaker RiskEventKind = "CIRCUIT_BREAKER"
	RiskEventMaxRisk        RiskEventKind = "MAX_RISK_REACHED"
	RiskEventRejectedEntry  RiskEventKind = "ENTRY_REJECTED"
)

// RiskEvent est un événement de risque horodaté, persisté pour audit.
type RiskEvent struct {
	Kind      RiskEventKind
	Detail    string
	Timestamp time.Time
}

// AlertLevel qualifie la sévérité d'une notification.
type AlertLevel string

const (
	AlertInfo     AlertLevel = "INFO"
	AlertWarning  AlertLevel = "WARNING"
	AlertCritical AlertLevel = "CRITICAL"
)
