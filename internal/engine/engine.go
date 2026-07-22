// Package engine contient le moteur de décision déterministe et la
// machine à états par symbole (IDLE, WAITING_ENTRY, IN_POSITION,
// WAITING_EXIT).
//
// Le moteur ÉMET des signaux ; il n'exécute rien lui-même. L'orchestrateur
// valide chaque signal auprès du risk manager puis pilote l'order manager,
// et informe le moteur des transitions (EntryPlaced, EntryFilled,
// ExitPlaced, Flat). Aucune logique Binance ici.
//
// Conditions d'entrée LONG (toutes requises, évaluées à la clôture de
// bougie) : croisement EMA rapide au-dessus de l'EMA lente, RSI > seuil
// d'entrée, prix > EMA lente, spread ≤ max, liquidité cumulée ≥ min.
// Le circuit breaker et l'absence de position sont vérifiés par le risk
// manager à la validation du signal (et le breaker est aussi consulté
// ici via un hook pour ne pas émettre de signaux pendant une suspension).
//
// Conditions de sortie (immédiates) : croisement EMA baissier ou
// RSI < seuil (à la clôture) ; stop loss, take profit, stop suiveur ou
// circuit breaker (à chaque tick).
package engine

import (
	"context"
	"log/slog"
	"sync"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// TrailingUpdater fait progresser le stop suiveur (risk.UpdateTrailing).
type TrailingUpdater func(pos types.Position, price float64) (stop float64, triggered bool)

// BreakerCheck indique si le circuit breaker global est actif.
type BreakerCheck func() bool

// Engine implémente types.StrategyEngine pour un symbole unique.
type Engine struct {
	cfg      config.EngineConfig
	symbol   string
	log      *slog.Logger
	trailing TrailingUpdater
	breaker  BreakerCheck

	mu       sync.Mutex
	state    types.EngineState
	agg      *candleAggregator
	emaFast  *EMA
	emaSlow  *EMA
	rsi      *RSI
	prevFast float64 // valeurs EMA de la bougie précédente (croisements)
	prevSlow float64
	hasPrev  bool
	pos      types.Position
}

var _ types.StrategyEngine = (*Engine)(nil)

// New construit le moteur en état IDLE.
func New(cfg config.EngineConfig, symbol string, trailing TrailingUpdater, breaker BreakerCheck, log *slog.Logger) *Engine {
	if trailing == nil {
		trailing = func(pos types.Position, _ float64) (float64, bool) { return pos.TrailingStop, false }
	}
	if breaker == nil {
		breaker = func() bool { return false }
	}
	return &Engine{
		cfg:      cfg,
		symbol:   symbol,
		log:      log.With("module", "engine", "symbol", symbol),
		trailing: trailing,
		breaker:  breaker,
		state:    types.StateIdle,
		agg:      newCandleAggregator(cfg.CandleInterval),
		emaFast:  NewEMA(cfg.EMAFastPeriod),
		emaSlow:  NewEMA(cfg.EMASlowPeriod),
		rsi:      NewRSI(cfg.RSIPeriod),
	}
}

// State retourne l'état courant de la machine à états.
func (e *Engine) State() types.EngineState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// EntryPlaced marque le placement d'un ordre d'entrée (IDLE → WAITING_ENTRY).
func (e *Engine) EntryPlaced() { e.transition(types.StateIdle, types.StateWaitingEntry) }

// EntryFilled marque l'exécution de l'entrée : la position (prix moyen,
// SL, TP) devient l'état de référence (WAITING_ENTRY → IN_POSITION).
func (e *Engine) EntryFilled(pos types.Position) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pos = pos
	e.setStateLocked(types.StateInPosition)
}

// ExitPlaced marque le placement de l'ordre de sortie (IN_POSITION →
// WAITING_EXIT).
func (e *Engine) ExitPlaced() { e.transition(types.StateInPosition, types.StateWaitingExit) }

// Flat remet le moteur à plat après clôture complète (→ IDLE).
func (e *Engine) Flat() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pos = types.Position{}
	e.setStateLocked(types.StateIdle)
}

// Position retourne la position suivie (stop suiveur inclus).
func (e *Engine) Position() types.Position {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pos
}

// OnEvent traite un événement de marché et retourne les signaux émis.
// Déterministe : aucun aléa, aucune horloge — seul le flux d'entrée
// gouverne les décisions.
func (e *Engine) OnEvent(ctx context.Context, ev types.MarketEvent) ([]types.Signal, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ev.Kind != types.EventTrade || ev.Tick == nil {
		return nil, nil // le carnet est déjà porté par les ticks enrichis
	}
	t := *ev.Tick

	e.mu.Lock()
	defer e.mu.Unlock()

	// 1. Protections intra-tick : SL / TP / stop suiveur / breaker.
	if e.state == types.StateInPosition {
		if sig := e.checkTickExitsLocked(t); sig != nil {
			return []types.Signal{*sig}, nil
		}
	}

	// 2. Indicateurs à la clôture de bougie.
	candle := e.agg.Update(t)
	if candle == nil {
		return nil, nil
	}
	prevReady := e.emaFast.Ready() && e.emaSlow.Ready()
	pf, ps := e.emaFast.Value(), e.emaSlow.Value()

	e.emaFast.Update(candle.Close)
	e.emaSlow.Update(candle.Close)
	e.rsi.Update(candle.Close)

	if !prevReady || !e.emaFast.Ready() || !e.emaSlow.Ready() || !e.rsi.Ready() {
		e.hasPrev = prevReady
		e.prevFast, e.prevSlow = pf, ps
		return nil, nil
	}

	crossUp := e.hasPrev && pf <= ps && e.emaFast.Value() > e.emaSlow.Value()
	crossDown := e.hasPrev && pf >= ps && e.emaFast.Value() < e.emaSlow.Value()
	e.hasPrev = true
	e.prevFast, e.prevSlow = pf, ps

	switch e.state {
	case types.StateIdle:
		if sig := e.checkEntryLocked(t, candle, crossUp); sig != nil {
			return []types.Signal{*sig}, nil
		}
	case types.StateInPosition:
		if crossDown {
			return []types.Signal{e.exitSignalLocked(t, "ema_cross_down")}, nil
		}
		if e.rsi.Value() < e.cfg.RSIExit {
			return []types.Signal{e.exitSignalLocked(t, "rsi_exit")}, nil
		}
	}
	return nil, nil
}

// checkEntryLocked évalue toutes les conditions d'entrée LONG.
func (e *Engine) checkEntryLocked(t types.Tick, candle *Candle, crossUp bool) *types.Signal {
	if !crossUp {
		return nil
	}
	if e.breaker() {
		e.log.Info("croisement haussier ignoré: circuit breaker actif")
		return nil
	}
	if e.rsi.Value() <= e.cfg.RSIEntry {
		e.log.Debug("entrée refusée: RSI insuffisant", "rsi", e.rsi.Value())
		return nil
	}
	if candle.Close <= e.emaSlow.Value() {
		e.log.Debug("entrée refusée: prix sous l'EMA lente",
			"close", candle.Close, "ema_slow", e.emaSlow.Value())
		return nil
	}
	if t.BestBid <= 0 || t.BestAsk <= 0 {
		return nil // carnet non synchronisé : pas de décision
	}
	spread := (t.BestAsk - t.BestBid) / t.BestBid
	if spread > e.cfg.MaxSpreadPct {
		e.log.Debug("entrée refusée: spread trop large", "spread", spread)
		return nil
	}
	if liq := t.BidVolume + t.AskVolume; liq < e.cfg.MinLiquidityBTC {
		e.log.Debug("entrée refusée: liquidité insuffisante", "liquidity", liq)
		return nil
	}

	e.log.Info("signal d'entrée LONG",
		"close", candle.Close, "rsi", e.rsi.Value(),
		"ema_fast", e.emaFast.Value(), "ema_slow", e.emaSlow.Value(),
		"spread", spread, "liquidity", t.BidVolume+t.AskVolume)
	return &types.Signal{
		Symbol:    e.symbol,
		Action:    types.ActionEnterLong,
		Price:     t.BestBid, // placement au meilleur prix acheteur
		Timestamp: t.Timestamp,
		Reason:    "ema_cross_up",
	}
}

// checkTickExitsLocked évalue les sorties immédiates sur chaque tick :
// stop loss, take profit, stop suiveur, circuit breaker global.
func (e *Engine) checkTickExitsLocked(t types.Tick) *types.Signal {
	if e.breaker() {
		sig := e.exitSignalLocked(t, "circuit_breaker")
		return &sig
	}
	if e.pos.StopLoss > 0 && t.Price <= e.pos.StopLoss {
		sig := e.exitSignalLocked(t, "stop_loss")
		return &sig
	}
	if e.pos.TakeProfit > 0 && t.Price >= e.pos.TakeProfit {
		sig := e.exitSignalLocked(t, "take_profit")
		return &sig
	}
	stop, triggered := e.trailing(e.pos, t.Price)
	e.pos.TrailingStop = stop
	if triggered {
		sig := e.exitSignalLocked(t, "trailing_stop")
		return &sig
	}
	return nil
}

// exitSignalLocked construit un signal de sortie immédiate.
func (e *Engine) exitSignalLocked(t types.Tick, reason string) types.Signal {
	e.log.Info("signal de sortie", "reason", reason, "price", t.Price,
		"stop_loss", e.pos.StopLoss, "take_profit", e.pos.TakeProfit,
		"trailing", e.pos.TrailingStop)
	return types.Signal{
		Symbol:    e.symbol,
		Action:    types.ActionExit,
		Price:     t.Price,
		Timestamp: t.Timestamp,
		Reason:    reason,
	}
}

// transition effectue un changement d'état attendu ; toute transition
// inattendue est journalisée (aide au diagnostic, jamais de panique).
func (e *Engine) transition(from, to types.EngineState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state != from {
		e.log.Warn("transition inattendue",
			"expected_from", string(from), "actual", string(e.state), "to", string(to))
	}
	e.setStateLocked(to)
}

func (e *Engine) setStateLocked(s types.EngineState) {
	if e.state != s {
		e.log.Info("changement d'état", "from", string(e.state), "to", string(s))
	}
	e.state = s
}
