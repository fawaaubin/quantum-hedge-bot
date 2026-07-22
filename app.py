import logging
import threading
import time
from functools import wraps

import numpy as np
import requests
from flask import Flask, jsonify, request, abort

from flask_socketio import SocketIO

from config import (
    FLASK_SECRET, API_TOKEN, CORS_ORIGINS,
    CAPITAL, MAX_DRAWDOWN, MAX_PER_PAIR_RISK, HEDGE_MODE, RISK_PCT,
    SYMBOLS, SYMBOL_SPOT, SYMBOL_FUTURES,
    DATA_FEED, POLL_INTERVAL,
    TELEGRAM_TOKEN, TELEGRAM_CHAT_ID, SLACK_WEBHOOK, EMAIL_CONFIG,
)
from database import init_db, get_db, save_trade, save_metrics, prometheus_metrics
from risk_manager import PortfolioRiskManager
from hedger import FuturesHedger
from strategies import Strategies
from alerts import AlertManager

# ───────────────────────────────
# LOGGING
# ───────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    handlers=[logging.FileHandler("system.log"), logging.StreamHandler()],
)
log = logging.getLogger("QuantumHedge")

# ───────────────────────────────
# FLASK + SOCKETIO
# ───────────────────────────────
app = Flask(__name__)
app.secret_key = FLASK_SECRET
socketio = SocketIO(app, cors_allowed_origins=CORS_ORIGINS, async_mode="threading")

# ───────────────────────────────
# RISK MANAGER + HEDGER + ALERTS
# ───────────────────────────────
rm = PortfolioRiskManager(CAPITAL, MAX_DRAWDOWN, MAX_PER_PAIR_RISK, HEDGE_MODE, RISK_PCT)
hedger = FuturesHedger(None, None, rm, SYMBOL_SPOT, SYMBOL_FUTURES)
alerts = AlertManager(TELEGRAM_TOKEN, TELEGRAM_CHAT_ID, SLACK_WEBHOOK, EMAIL_CONFIG)

# Anti-spam d'alertes : on ne réémet pas la même alerte critique en boucle.
_last_dd_alert = 0.0


# ───────────────────────────────
# AUTH (protège les endpoints d'écriture)
# ───────────────────────────────
def require_api_token(fn):
    @wraps(fn)
    def wrapper(*args, **kwargs):
        if not API_TOKEN:
            log.warning("Refus /manual_trade : API_TOKEN non configuré côté serveur.")
            abort(403, description="Manual trading disabled: server API_TOKEN not set.")
        provided = request.headers.get("X-API-Key", "")
        # Comparaison à temps constant contre le timing attack.
        import hmac
        if not hmac.compare_digest(provided, API_TOKEN):
            abort(401, description="Invalid or missing API key.")
        return fn(*args, **kwargs)
    return wrapper


# ───────────────────────────────
# DATA FEED
# ───────────────────────────────
_binance_map = {}  # cache symbol -> dernier prix connu (fallback réseau)


def fetch_price(symbol):
    """Récupère (price, volume) selon la source configurée."""
    if DATA_FEED == "binance":
        try:
            r = requests.get(
                "https://api.binance.com/api/v3/ticker/24hr",
                params={"symbol": symbol}, timeout=5,
            )
            r.raise_for_status()
            d = r.json()
            price = float(d["lastPrice"])
            volume = float(d["volume"])
            _binance_map[symbol] = price
            return price, volume
        except Exception as e:
            log.warning("Feed Binance indisponible pour %s (%s) — fallback.", symbol, e)
            base = _binance_map.get(symbol, 60000.0)
            return float(np.random.normal(base, base * 0.005)), float(abs(np.random.normal(1000, 200)))
    # Mode simulation (paper-trading) : marche aléatoire par symbole.
    base = _binance_map.get(symbol, 60000.0)
    price = float(np.random.normal(base, base * 0.01))
    _binance_map[symbol] = price
    return price, float(abs(np.random.normal(1000, 200)))


# ───────────────────────────────
# TRADING LOOP MULTI‑PAIRES
# ───────────────────────────────
def trading_loop(symbols):
    global _last_dd_alert
    prices = {s: [] for s in symbols}
    volumes = {s: [] for s in symbols}

    while True:
        try:
            for sym in symbols:
                price, volume = fetch_price(sym)
                prices[sym].append(price)
                volumes[sym].append(volume)
                if len(prices[sym]) > 300:
                    prices[sym].pop(0)
                if len(volumes[sym]) > 300:
                    volumes[sym].pop(0)

                # 1) Gestion des sorties : SL/TP des positions ouvertes.
                for c in rm.check_exits(sym, price):
                    hedger.execute_spot_order(
                        "SELL" if c["side"] == "BUY" else "BUY",
                        c["qty"], price, f"exit_{c['reason']}")
                    save_trade(sym, f"CLOSE_{c['side']}", c["qty"], price, c["pnl"])
                    log.info("Sortie %s %s | %s | PnL=%.2f",
                             sym, c["side"], c["reason"], c["pnl"])
                    socketio.emit("position_closed", c)

                # Signaux
                score, signals = Strategies.aggregate(prices[sym], volumes[sym])
                log.info("%s | Score=%s | Signals=%s", sym, score, signals)

                # Coupe-circuit : aucune nouvelle prise de position si DD max atteint.
                if not rm.can_trade():
                    socketio.emit("price_update", {
                        "symbol": sym, "price": price, "score": score,
                        "signals": signals, "halted": True,
                    })
                    continue

                # Décision : une seule position ouverte par paire (pas d'empilement).
                already_open = bool(rm.positions.get(sym))
                if score > 2 and not already_open:
                    sl = price * 0.98
                    tp = price * 1.04
                    qty = rm.calc_position_size(sym, price, sl)
                    if qty > 0:
                        hedger.execute_spot_order("BUY", qty, price, "aggregate", sl=sl, tp=tp)
                        hedger.auto_hedge(qty, price, spot_side="BUY")
                        rm.add_position(sym, qty, price, "BUY", sl=sl, tp=tp)
                        save_trade(sym, "BUY", qty, price)
                elif score < -2 and not already_open:
                    sl = price * 1.02
                    tp = price * 0.96
                    qty = rm.calc_position_size(sym, price, sl)
                    if qty > 0:
                        hedger.execute_spot_order("SELL", qty, price, "aggregate", sl=sl, tp=tp)
                        hedger.auto_hedge(qty, price, spot_side="SELL")
                        rm.add_position(sym, qty, price, "SELL", sl=sl, tp=tp)
                        save_trade(sym, "SELL", qty, price)

                # Push temps réel
                socketio.emit("price_update", {
                    "symbol": sym, "price": price, "score": score, "signals": signals,
                })

                # Alertes drawdown (anti-spam : 1 max / 5 min)
                if rm.drawdown > MAX_DRAWDOWN and (time.time() - _last_dd_alert) > 300:
                    alerts.send_alert(f"⚠️ Drawdown critique {rm.drawdown:.2%} (halte trading)")
                    _last_dd_alert = time.time()

            # Persiste un snapshot de métriques par cycle (Prometheus/Grafana).
            snap = rm.snapshot()
            trades_total = sum(len(v) for v in rm.positions.values())
            save_metrics(snap["capital"], snap["drawdown"], trades_total, snap["realized_pnl"])

            time.sleep(POLL_INTERVAL)
        except Exception as e:  # la boucle ne doit jamais mourir
            log.exception("Loop error")
            try:
                alerts.send_alert(f"❌ Erreur critique boucle: {e}")
            except Exception:
                pass
            time.sleep(POLL_INTERVAL)


# ───────────────────────────────
# ENDPOINTS REST
# ───────────────────────────────
@app.route("/health")
def health():
    return jsonify({"status": "ok", "feed": DATA_FEED})


@app.route("/status")
def status():
    return jsonify(rm.snapshot())


@app.route("/trades")
def trades():
    with get_db() as conn:
        rows = conn.execute(
            "SELECT * FROM trades ORDER BY timestamp DESC LIMIT 50"
        ).fetchall()
        return jsonify([dict(r) for r in rows])


@app.route("/metrics")
def metrics():
    # Endpoint scrapé par Prometheus (format texte).
    return prometheus_metrics()


@app.route("/manual_trade", methods=["POST"])
@require_api_token
def manual_trade():
    data = request.get_json(silent=True) or {}
    try:
        side = str(data["side"]).upper()
        qty = float(data["qty"])
        price = float(data["price"])
    except (KeyError, TypeError, ValueError):
        abort(400, description="Champs requis: side, qty, price (numériques).")

    if side not in ("BUY", "SELL"):
        abort(400, description="side doit valoir BUY ou SELL.")
    if qty <= 0 or price <= 0:
        abort(400, description="qty et price doivent être strictement positifs.")

    sl = data.get("sl")
    tp = data.get("tp")
    symbol = data.get("symbol", SYMBOL_SPOT)

    order = hedger.execute_spot_order(side, qty, price, "manual", sl=sl, tp=tp)
    save_trade(symbol, side, qty, price)
    rm.add_position(symbol, qty, price, side,
                    sl=float(sl) if sl is not None else None,
                    tp=float(tp) if tp is not None else None)
    return jsonify({"status": "ok", "order": order})


# ───────────────────────────────
# MAIN
# ───────────────────────────────
if __name__ == "__main__":
    init_db()
    symbols = SYMBOLS if SYMBOLS else [SYMBOL_SPOT]
    t = threading.Thread(target=trading_loop, args=(symbols,), daemon=True)
    t.start()
    log.info("🚀 Quantum Hedge Bot EDGE lancé | feed=%s | pairs=%s", DATA_FEED, symbols)
    socketio.run(app, host="0.0.0.0", port=8080, allow_unsafe_werkzeug=True)
