# Quantum Hedge Bot 🚀

Un bot de trading crypto **modulaire**, pensé pour être robuste, sécurisé et testable.
Il combine **stratégies techniques**, **risk management**, **hedging Spot/Futures**,
**dashboard web temps réel** et **monitoring Prometheus/Grafana**.

> ⚠️ **Avertissement.** Ce logiciel est fourni à des fins éducatives. Le trading de
> crypto-actifs comporte un risque de perte en capital. Utilisez d'abord le mode
> `DATA_FEED=sim` (paper-trading) et le testnet avant tout capital réel.

---

## ✨ Fonctionnalités

- Multi-paires (BTC, ETH, BNB…)
- Stratégies : RSI, MACD, Bollinger, EMA cross, volume surge (agrégation par vote)
- Risk management : sizing basé sur le risque, plafond par paire, **coupe-circuit drawdown**
- Hedging automatique Spot ↔ Futures
- API REST : `/health`, `/status`, `/trades`, `/metrics`, `/manual_trade` (protégé par jeton)
- Dashboard web temps réel (Chart.js + SocketIO)
- Alertes Telegram / Slack / Email
- Backtesting **event-driven avec P&L réel** (win rate, profit factor, max drawdown)
- CI/CD GitHub Actions (tests + build Docker)
- Déploiement Docker + monitoring Prometheus/Grafana

---

## 📦 Installation

```bash
git clone https://github.com/<votre-compte>/quantum-hedge-bot.git
cd quantum-hedge-bot
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env        # puis renseignez FLASK_SECRET, API_TOKEN, etc.
```

## ▶️ Lancer

```bash
# Paper-trading (aucun réseau, prix simulés)
DATA_FEED=sim python app.py

# Prix réels via l'API publique Binance
DATA_FEED=binance python app.py
```

Le bot démarre sur `http://localhost:8080`. Ouvrez `dashboard.html` pour le suivi live.

## 🧪 Tests & backtest

```bash
pytest tests/ -v
python backtest.py
```

## 🐳 Docker

```bash
docker compose up --build              # dev / paper-trading
docker compose -f docker-compose.prod.yml up -d   # prod + Prometheus/Grafana
```

---

## 🔒 Sécurité

- **`FLASK_SECRET` obligatoire** en prod (généré aléatoirement sinon, non persistant).
- **`/manual_trade` protégé** par en-tête `X-API-Key` (comparaison à temps constant) ;
  l'endpoint est **désactivé** tant qu'`API_TOKEN` n'est pas défini.
- Conteneur Docker en **utilisateur non-root** + `HEALTHCHECK`.
- Secrets via `.env` (jamais versionné — voir `.gitignore`).
- `CORS_ORIGINS` restreignable (évitez `*` en prod).

Exemple d'appel manuel authentifié :

```bash
curl -X POST http://localhost:8080/manual_trade \
  -H "X-API-Key: $API_TOKEN" -H "Content-Type: application/json" \
  -d '{"side":"BUY","qty":0.01,"price":60000,"symbol":"BTCUSDT"}'
```

---

## 🗂️ Architecture

| Module | Rôle |
|---|---|
| `app.py` | API Flask + SocketIO, boucle de trading multi-paires |
| `strategies.py` | Indicateurs + agrégation des signaux |
| `risk_manager.py` | Sizing, drawdown, coupe-circuit |
| `hedger.py` | Exécution Spot/Futures + auto-hedge |
| `alerts.py` | Telegram / Slack / Email |
| `database.py` | Persistance SQLite + export Prometheus |
| `backtest.py` | Backtesting event-driven (P&L réel) |
