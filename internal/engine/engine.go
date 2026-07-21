// Package engine contient le moteur de décision et la machine à états
// par symbole (IDLE, WAITING_ENTRY, IN_POSITION, WAITING_EXIT).
//
// Phase 1 : squelette sans aucune logique de stratégie réelle.
package engine

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Engine implémente types.StrategyEngine pour un symbole unique.
type Engine struct {
	symbol string
	log    *slog.Logger
	state  atomic.Value // types.EngineState
}

var _ types.StrategyEngine = (*Engine)(nil)

// New construit le moteur en état IDLE.
func New(symbol string, log *slog.Logger) *Engine {
	e := &Engine{symbol: symbol, log: log.With("module", "engine", "symbol", symbol)}
	e.state.Store(types.StateIdle)
	return e
}

// OnEvent sera implémenté en Phase 5 (EMA 9/21, RSI 14, conditions
// d'entrée LONG et de sortie déterministes).
func (e *Engine) OnEvent(ctx context.Context, ev types.MarketEvent) ([]types.Signal, error) {
	return nil, types.ErrNotImplemented
}

// State retourne l'état courant de la machine à états.
func (e *Engine) State() types.EngineState {
	return e.state.Load().(types.EngineState)
}
