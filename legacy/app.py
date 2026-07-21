import logging, threading, time
import numpy as np
from flask import Flask, jsonify, request
from flask_socketio import SocketIO

from config import *
from database import init_db, get_db
from risk_manager import PortfolioRiskManager
from hedger import FuturesHedger
from strategies import Strategies
from alerts import AlertManager

# ───────────────────────────────
# LOGGING
# ───────────────────────────────
logging.basicConfig(level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    handlers=[logging.FileHandler("system.log"), logging.StreamHandler()])
log = logging.getLogger("QuantumHedge")

# ───────────────────────────────
# FLASK + SOCKETIO
# ───────────────────────────────
app = Flask(__name__)
app.secret_key = FLASK_SECRET
socketio = SocketIO(app, cors_allowed_origins="*", async_mode="threading")

# ───────────────────────────────
# RISK MANAGER + HEDGER + ALERTS
# ───────────────────────────────
rm = PortfolioRiskManager(CAPITAL, MAX_DRAWDOWN, MAX_PER_PAIR_RISK, HEDGE_MODE, RISK_PCT)
hedger = FuturesHedger(None, None, rm, SYMBOL_SPOT, SYMBOL_FUTURES)
alerts = AlertManager(TELEGRAM_TOKEN, TELEGRAM_CHAT_ID)

# ───────────────────────────────
# TRADING LOOP MULTI‑PAIRES
# ───────────────────────────────
def trading_loop(symbols):
    prices = {s: [] for s in symbols}
    volumes = {s: [] for s in symbols}
    while True:
        try:
            for sym in symbols:
                # Simulation (remplacer par API Binance en prod)
                price = float(np.random.normal(60000, 500))
                volume = float(np.random.normal(1000, 200))
                prices[sym].append(price)
                volumes[sym].append(volume)
                if len(prices[sym]) > 300: prices[sym].pop(0)
                if len(volumes[sym]) > 300: volumes[sym].pop(0)

                # Signaux
                score, signals = Strategies.aggregate(prices[sym], volumes[sym])
                log.info(f"{sym} | Score={score:.2f} | Signals={signals}")

                # Décision
                if score > 2:
                    sl = price * 0.98
                    tp = price * 1.04
                    qty = rm.calc_position_size(sym, price, sl)
                    hedger.execute_spot_order("BUY", qty, price, "aggregate", sl=sl, tp=tp)
                    hedger.auto_hedge(qty, price)
                    rm.add_position(sym, qty, price, "BUY")
                elif score < -2:
                    sl = price * 1.02
                    tp = price * 0.96
                    qty = rm.calc_position_size(sym, price, sl)
                    hedger.execute_spot_order("SELL", qty, price, "aggregate", sl=sl, tp=tp)
                    hedger.auto_hedge(qty, price)
                    rm.add_position(sym, qty, price, "SELL")

                # Push temps réel
                socketio.emit("price_update", {"symbol": sym, "price": price, "score": score, "signals": signals})

                # Alertes
                if rm.drawdown > MAX_DRAWDOWN:
                    alerts.send_alert(f"⚠️ Drawdown critique {rm.drawdown:.2%} sur {sym}")

            time.sleep(5)
        except Exception as e:
            log.error(f"Loop error: {e}")
            alerts.send_alert(f"❌ Erreur critique: {e}")
            time.sleep(5)

# ───────────────────────────────
# ENDPOINTS REST
# ───────────────────────────────
@app.route("/status")
def status():
    return jsonify({"capital": rm.capital, "drawdown": rm.drawdown, "positions": rm.positions})

@app.route("/trades")
def trades():
    with get_db() as conn:
        rows = conn.execute("SELECT * FROM trades ORDER BY timestamp DESC LIMIT 50").fetchall()
        return jsonify([dict(r) for r in rows])

@app.route("/manual_trade", methods=["POST"])
def manual_trade():
    data = request.json
    side = data.get("side")
    qty = float(data.get("qty"))
    price = float(data.get("price"))
    sl = data.get("sl")
    tp = data.get("tp")
    symbol = data.get("symbol", SYMBOL_SPOT)
    hedger.execute_spot_order(side, qty, price, "manual", sl=sl, tp=tp)
    return jsonify({"status":"ok"})

# ───────────────────────────────
# MAIN
# ───────────────────────────────
if __name__ == "__main__":
    init_db()
    symbols = [SYMBOL_SPOT, "ETHUSDT", "BNBUSDT"]  # multi‑paires
    t = threading.Thread(target=trading_loop, args=(symbols,), daemon=True)
    t.start()
    log.info("🚀 Quantum Hedge Bot EDGE lancé")
    socketio.run(app, host="0.0.0.0", port=8080)
