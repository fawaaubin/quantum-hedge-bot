// Package store persiste l'état du bot dans SQLite (mode WAL, écrivain
// unique protégé par mutex).
//
// Phase 1 : squelette sans aucune persistance réelle.
package store

import (
	"context"
	"log/slog"
	"sync"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// Store implémente types.StateStore.
type Store struct {
	cfg config.StoreConfig
	log *slog.Logger

	// mu protégera l'écrivain SQLite unique (Phase 6).
	mu sync.Mutex
}

var _ types.StateStore = (*Store)(nil)

// New construit le store. L'ouverture SQLite (WAL) arrive en Phase 6.
func New(cfg config.StoreConfig, log *slog.Logger) *Store {
	return &Store{cfg: cfg, log: log.With("module", "store")}
}

func (s *Store) SavePosition(ctx context.Context, pos types.Position) error {
	return types.ErrNotImplemented
}

func (s *Store) OpenPositions(ctx context.Context) ([]types.Position, error) {
	return nil, types.ErrNotImplemented
}

func (s *Store) SaveOrder(ctx context.Context, ord types.Order) error {
	return types.ErrNotImplemented
}

func (s *Store) SaveTrade(ctx context.Context, res types.TradeResult) error {
	return types.ErrNotImplemented
}

func (s *Store) SaveBotState(ctx context.Context, st types.BotState) error {
	return types.ErrNotImplemented
}

func (s *Store) LoadBotState(ctx context.Context) (types.BotState, error) {
	return types.BotState{}, types.ErrNotImplemented
}

func (s *Store) SaveRiskEvent(ctx context.Context, ev types.RiskEvent) error {
	return types.ErrNotImplemented
}

func (s *Store) Close(ctx context.Context) error {
	return nil
}
