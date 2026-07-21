// Package backtest rejoue des données historiques tick par tick via
// l'interface types.DataFeed, avec les mêmes règles de risque que le live.
//
// Phase 1 : squelette sans aucune logique de rejeu réelle.
package backtest

import (
	"context"
	"log/slog"

	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// CSVFeed rejouera un fichier CSV/JSON de ticks historiques.
type CSVFeed struct {
	path string
	log  *slog.Logger
}

var _ types.DataFeed = (*CSVFeed)(nil)

// NewCSVFeed construit un feed pointant vers un fichier de données.
func NewCSVFeed(path string, log *slog.Logger) *CSVFeed {
	return &CSVFeed{path: path, log: log.With("module", "backtest")}
}

// Next sera implémenté avec le module de backtest (rejeu chronologique).
func (f *CSVFeed) Next(ctx context.Context) (types.Tick, error) {
	return types.Tick{}, types.ErrNotImplemented
}

// Reset sera implémenté avec le module de backtest.
func (f *CSVFeed) Reset() error {
	return types.ErrNotImplemented
}
