// Package gateway gère les WebSocket publics Binance (flux trade et
// depth), maintient un carnet d'ordres local synchronisé et publie des
// événements de marché normalisés sur un channel bufferisé.
//
// Résilience : reconnexion automatique avec backoff exponentiel + jitter,
// resynchronisation du carnet (nouveau snapshot REST) sur tout gap de
// séquence, réabonnement implicite via l'URL de flux combiné.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/fawaaubin/quantum-hedge-bot/internal/config"
	"github.com/fawaaubin/quantum-hedge-bot/pkg/types"
)

// readTimeout borne l'attente d'un message WebSocket : le flux depth
// publie ~10 messages/s, un silence prolongé signifie une connexion morte.
const readTimeout = 90 * time.Second

// handshakeTimeout borne l'établissement de la connexion WebSocket.
const handshakeTimeout = 10 * time.Second

// SnapshotFetcher fournit le snapshot REST initial du carnet d'ordres.
// Implémenté par *binance.Client ; interface locale pour la testabilité.
type SnapshotFetcher interface {
	DepthSnapshot(ctx context.Context, symbol string, limit int) (types.OrderBook, error)
}

// Gateway implémente types.MarketDataSource pour un symbole unique.
type Gateway struct {
	cfg    config.GatewayConfig
	wsBase string
	symbol string
	snap   SnapshotFetcher
	log    *slog.Logger

	book   *localBook
	events chan types.MarketEvent

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool

	// Compteurs d'observabilité (exposés à Prometheus en Phase 6).
	dropped    atomic.Int64
	resyncs    atomic.Int64
	reconnects atomic.Int64
}

var _ types.MarketDataSource = (*Gateway)(nil)

// New construit une gateway pour un symbole unique.
func New(cfg config.GatewayConfig, wsBase, symbol string, snap SnapshotFetcher, log *slog.Logger) *Gateway {
	return &Gateway{
		cfg:    cfg,
		wsBase: strings.TrimRight(wsBase, "/"),
		symbol: symbol,
		snap:   snap,
		log:    log.With("module", "gateway", "symbol", symbol),
		book:   newLocalBook(symbol),
		events: make(chan types.MarketEvent, cfg.EventBufferSize),
	}
}

// Start lance la boucle de connexion en arrière-plan. L'annulation du
// contexte parent arrête la gateway.
func (g *Gateway) Start(ctx context.Context) error {
	if !g.started.CompareAndSwap(false, true) {
		return errors.New("gateway déjà démarrée")
	}
	runCtx, cancel := context.WithCancel(ctx)
	g.cancel = cancel
	g.wg.Add(1)
	go g.run(runCtx)
	return nil
}

// Events expose le channel bufferisé d'événements de marché. Le channel
// est fermé à l'arrêt de la gateway.
func (g *Gateway) Events() <-chan types.MarketEvent {
	return g.events
}

// OrderBook retourne une copie de l'état courant du carnet local.
func (g *Gateway) OrderBook(ctx context.Context, symbol string) (types.OrderBook, error) {
	if symbol != g.symbol {
		return types.OrderBook{}, fmt.Errorf("symbole inconnu: %s", symbol)
	}
	if !g.book.isSynced() {
		return types.OrderBook{}, errNotSynced
	}
	return g.book.snapshot(), nil
}

// Stop arrête la gateway et attend la fin des goroutines, borné par ctx.
func (g *Gateway) Stop(ctx context.Context) error {
	if g.cancel == nil {
		return nil
	}
	g.cancel()
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("arrêt de la gateway: %w", ctx.Err())
	}
}

// Dropped retourne le nombre d'événements perdus (channel plein).
func (g *Gateway) Dropped() int64 { return g.dropped.Load() }

// Resyncs retourne le nombre de resynchronisations du carnet.
func (g *Gateway) Resyncs() int64 { return g.resyncs.Load() }

// Reconnects retourne le nombre de reconnexions WebSocket.
func (g *Gateway) Reconnects() int64 { return g.reconnects.Load() }

// streamURL construit l'URL du flux combiné trade + diff depth.
func (g *Gateway) streamURL() string {
	s := strings.ToLower(g.symbol)
	speed := g.cfg.DepthStreamSpeed
	if speed == "" {
		speed = "100ms"
	}
	return fmt.Sprintf("%s/stream?streams=%s@trade/%s@depth@%s", g.wsBase, s, s, speed)
}

// run est la boucle de vie : connexion, service, reconnexion avec
// backoff exponentiel + jitter jusqu'à annulation du contexte.
func (g *Gateway) run(ctx context.Context) {
	defer g.wg.Done()
	defer close(g.events)

	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}

		start := time.Now()
		err := g.serve(ctx)
		if ctx.Err() != nil {
			return
		}

		// Une connexion qui a tenu durablement remet le backoff à zéro.
		if time.Since(start) > time.Minute {
			attempt = 0
		}
		delay := nextBackoff(attempt, g.cfg.BackoffInitial, g.cfg.BackoffMax)
		attempt++
		g.reconnects.Add(1)
		g.log.Warn("connexion perdue, reconnexion planifiée",
			"err", err, "uptime", time.Since(start).String(), "delay", delay.String())

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// serve établit une connexion, synchronise le carnet puis consomme les
// messages jusqu'à erreur ou annulation.
func (g *Gateway) serve(ctx context.Context) error {
	g.book.reset()

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: handshakeTimeout,
	}
	conn, _, err := dialer.DialContext(ctx, g.streamURL(), nil)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}
	defer conn.Close()

	// Débloque ReadMessage à l'annulation du contexte.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-watchDone:
		}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPingHandler(func(appData string) error {
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	// Snapshot APRÈS ouverture du flux : les événements émis pendant la
	// récupération restent en attente côté connexion, puis sont soit
	// obsolètes (ignorés), soit appliqués ; un gap déclenche un resync.
	if err := g.bootstrap(ctx); err != nil {
		return err
	}

	g.log.Info("flux connecté et carnet synchronisé", "last_update_id", g.book.lastID())

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("lecture websocket: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

		if err := g.handleMessage(raw); err != nil {
			if errors.Is(err, errGap) {
				g.resyncs.Add(1)
				g.log.Warn("gap de séquence détecté, resynchronisation du carnet")
				if err := g.bootstrap(ctx); err != nil {
					return err
				}
				continue
			}
			g.log.Warn("message ignoré", "err", err)
		}
	}
}

// bootstrap (re)construit le carnet local depuis un snapshot REST.
// Tout état en attente est abandonné : le snapshot devient la nouvelle
// référence de synchronisation.
func (g *Gateway) bootstrap(ctx context.Context) error {
	g.book.reset()

	snapCtx, cancel := context.WithTimeout(ctx, g.cfg.SnapshotTimeout)
	defer cancel()

	ob, err := g.snap.DepthSnapshot(snapCtx, g.symbol, g.cfg.SnapshotDepth)
	if err != nil {
		return fmt.Errorf("snapshot du carnet: %w", err)
	}
	g.book.applySnapshot(ob)
	return nil
}

// handleMessage parse un message brut, met à jour le carnet et publie
// l'événement normalisé. Retourne errGap si une resynchronisation est
// nécessaire.
func (g *Gateway) handleMessage(raw []byte) error {
	ev, err := parseCombined(raw)
	if err != nil {
		return err
	}

	switch {
	case ev.depth != nil:
		applied, err := g.book.applyDiff(*ev.depth)
		if err != nil {
			return err
		}
		if applied {
			g.publish(types.MarketEvent{Kind: types.EventDepth, Depth: ev.depth})
		}
		return nil

	case ev.trade != nil:
		// Enrichissement du tick avec l'état du carnet local.
		if bid, ask, bidVol, askVol, ok := g.book.top(20); ok {
			ev.trade.BestBid = bid.Price
			ev.trade.BestAsk = ask.Price
			ev.trade.BidVolume = bidVol
			ev.trade.AskVolume = askVol
			ev.trade.LastUpdateID = g.book.lastID()
		}
		g.publish(types.MarketEvent{Kind: types.EventTrade, Tick: ev.trade})
		return nil
	}
	return nil
}

// publish envoie l'événement sans jamais bloquer la boucle de lecture :
// si le channel est plein, l'événement est compté comme perdu.
func (g *Gateway) publish(ev types.MarketEvent) {
	select {
	case g.events <- ev:
	default:
		g.dropped.Add(1)
	}
}
