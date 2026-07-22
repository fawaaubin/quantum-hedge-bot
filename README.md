# Quantum Hedge Bot (v2 — Go)

Bot de trading **Binance Spot Testnet** (BTCUSDT uniquement), écrit en Go.
Architecture *modular monolith*, développement par phases strictes : chaque
phase est compilable, testée et validable indépendamment.

> L'ancien prototype Python (non fonctionnel) est archivé dans [`legacy/`](legacy/).

## État des phases

| Phase | Contenu | État |
|---|---|---|
| 1 | Architecture, interfaces, config, logger | ✅ livrée |
| 2 | Gateway temps réel (WS trade + depth, carnet local, resync, backoff) | ✅ livrée |
| 3 | Trading signé (ordres LIMIT GTC, HMAC, filtres, rate limiting) | ✅ livrée |
| 4 | Risk manager (sizing, SL/TP, circuit breaker) | ✅ livrée |
| 5 | Engine (EMA 9/21, RSI 14, machine à états) | ✅ livrée |
| 6 | Production (SQLite WAL, /health, /metrics, Telegram, Docker) | ⏳ |

## Structure

```
cmd/bot/          Point d'entrée
internal/gateway/       WebSocket publics, carnet d'ordres local
internal/engine/        Moteur de décision (machine à états)
internal/ordermanager/  Exécution et suivi des ordres
internal/risk/          Sizing, SL/TP, circuit breaker
internal/store/         Persistance SQLite (WAL)
internal/monitor/       /health, /metrics, alertes
internal/binance/       Client REST/WS bas niveau, filtres
internal/backtest/      Rejeu de données historiques
internal/config/        Configuration YAML + validation
internal/logger/        slog structuré (secrets masqués)
pkg/types/              Types de domaine et interfaces partagées
```

## Démarrage

```bash
# 1. Configuration
cp config.yaml.example config.yaml

# 2. Secrets (jamais dans le YAML) — requis à partir de la Phase 3
export BINANCE_API_KEY="..."
export BINANCE_API_SECRET="..."

# 3. Build + tests
go build ./...
go test -race ./...

# 4. Lancement
go run ./cmd/bot -config config.yaml
```

Le bot se connecte aux flux publics `btcusdt@trade` et
`btcusdt@depth@100ms` du testnet, maintient un carnet d'ordres local
synchronisé (snapshot REST + updates incrémentales, resynchronisation
automatique sur gap de séquence) et journalise l'état du marché toutes
les 10 s. Avec des clés API, il réconcilie l'état des ordres au
démarrage ; sans clés, il reste en mode observation. Aucun ordre n'est
envoyé tant que le moteur de décision (Phase 5) n'est pas actif.

### Test d'intégration testnet (optionnel)

```bash
BINANCE_API_KEY=... BINANCE_API_SECRET=... \
  go test -tags integration -run TestIntegration ./internal/ordermanager/
```

Place un ordre LIMIT GTC 20 % sous le marché (non exécutable), vérifie
`openOrders` et l'anti-double-ordre, puis annule.

## Notes de conception

- **Flux depth** : le carnet local est maintenu via le *diff stream*
  (`@depth@100ms`) et l'algorithme officiel Binance de synchronisation
  (`U`/`u`/`lastUpdateId`). Le flux partiel `depth20@100ms` ne porte pas
  de numéros de séquence et ne permet pas un carnet fiable.
- **Événements** : publiés sur un channel bufferisé (256) ; la boucle de
  lecture WebSocket n'est jamais bloquée — en cas de saturation, les
  événements sont comptés comme perdus (métrique exposée en Phase 6).
- **Résilience** : reconnexion avec backoff exponentiel + jitter (borné,
  configurable), resynchronisation du carnet à chaque reconnexion ou gap.
- **Proxy** : les transports HTTP et WebSocket honorent
  `HTTPS_PROXY`/`HTTP_PROXY` (`ProxyFromEnvironment`).
