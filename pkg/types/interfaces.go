package types

import "context"

// DataFeed est l'interface commune de rejeu de données historiques
// (backtest) : les ticks sont fournis chronologiquement.
type DataFeed interface {
	// Next retourne le tick suivant. Retourne io.EOF (ou une erreur
	// dédiée du module) lorsque le flux est épuisé.
	Next(ctx context.Context) (Tick, error)
	// Reset repositionne le flux au début.
	Reset() error
}

// MarketDataSource est la source de données de marché temps réel
// (WebSocket publics + snapshot REST), implémentée par la gateway.
type MarketDataSource interface {
	// Start ouvre les connexions et démarre la publication d'événements.
	// L'annulation du contexte arrête la source.
	Start(ctx context.Context) error
	// Events expose le channel bufferisé d'événements de marché.
	Events() <-chan MarketEvent
	// OrderBook retourne une copie de l'état courant du carnet local.
	OrderBook(ctx context.Context, symbol string) (OrderBook, error)
	// Stop arrête proprement la source (drainage, fermeture des connexions).
	Stop(ctx context.Context) error
}

// OrderExecutor exécute, annule, remplace et suit les ordres.
type OrderExecutor interface {
	PlaceOrder(ctx context.Context, req OrderRequest) (Order, error)
	CancelOrder(ctx context.Context, symbol string, orderID int64) error
	// ReplaceOrder annule l'ordre existant puis en place un nouveau,
	// après vérification de l'état réel de l'ordre.
	ReplaceOrder(ctx context.Context, symbol string, orderID int64, req OrderRequest) (Order, error)
	OpenOrders(ctx context.Context, symbol string) ([]Order, error)
	// Reconcile réconcilie l'état local avec openOrders + account au
	// démarrage pour garantir l'absence de double ordre.
	Reconcile(ctx context.Context) error
}

// RiskManager valide les entrées, dimensionne les positions et calcule
// les niveaux de sortie.
type RiskManager interface {
	// PositionSize calcule la taille de position pour une entrée donnée
	// (prix d'entrée et stop loss), en respectant le risque par trade.
	PositionSize(ctx context.Context, entry, stopLoss, capital float64) (float64, error)
	// ValidateEntry vérifie qu'un signal d'entrée respecte toutes les
	// contraintes de risque (risque max, nombre de positions, breaker).
	ValidateEntry(ctx context.Context, sig Signal) error
	// ExitLevels calcule stop loss, take profit et stop suiveur pour
	// une position donnée.
	ExitLevels(ctx context.Context, pos Position) (ExitLevels, error)
	// RecordTradeResult alimente l'historique utilisé par le circuit breaker.
	RecordTradeResult(ctx context.Context, res TradeResult) error
	// CircuitBreakerActive indique si le trading est suspendu.
	CircuitBreakerActive(ctx context.Context) (bool, error)
}

// StateStore persiste positions, ordres, trades, état du bot et
// événements de risque (SQLite WAL en Phase 6).
type StateStore interface {
	SavePosition(ctx context.Context, pos Position) error
	OpenPositions(ctx context.Context) ([]Position, error)
	SaveOrder(ctx context.Context, ord Order) error
	SaveTrade(ctx context.Context, res TradeResult) error
	SaveBotState(ctx context.Context, st BotState) error
	LoadBotState(ctx context.Context) (BotState, error)
	SaveRiskEvent(ctx context.Context, ev RiskEvent) error
	Close(ctx context.Context) error
}

// StrategyEngine est le moteur de décision : il consomme des événements
// de marché et émet des signaux déterministes.
type StrategyEngine interface {
	// OnEvent traite un événement de marché et retourne les signaux
	// éventuels (entrée/sortie). Déterministe : même entrée, même sortie.
	OnEvent(ctx context.Context, ev MarketEvent) ([]Signal, error)
	// State expose l'état courant de la machine à états.
	State() EngineState
}

// Notifier envoie des alertes de manière non bloquante (Telegram en
// Phase 6). Les erreurs d'envoi ne doivent jamais interrompre le trading.
type Notifier interface {
	Notify(ctx context.Context, level AlertLevel, message string) error
}
