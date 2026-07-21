import sqlite3
import logging
from contextlib import contextmanager
from flask import Response

log = logging.getLogger("QuantumHedgeDB")

DB_FILE = "quantum_hedge.db"

# ───────────────────────────────
# INIT DB
# ───────────────────────────────
def init_db():
    conn = sqlite3.connect(DB_FILE)
    c = conn.cursor()
    c.execute("""
        CREATE TABLE IF NOT EXISTS trades (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
            symbol TEXT,
            side TEXT,
            qty REAL,
            price REAL,
            pnl REAL DEFAULT 0
        )
    """)
    c.execute("""
        CREATE TABLE IF NOT EXISTS metrics (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
            capital REAL,
            drawdown REAL,
            trades_total INTEGER,
            pnl REAL
        )
    """)
    conn.commit()
    conn.close()
    log.info("Database initialized")

# ───────────────────────────────
# CONTEXT MANAGER
# ───────────────────────────────
@contextmanager
def get_db():
    conn = sqlite3.connect(DB_FILE)
    conn.row_factory = sqlite3.Row
    try:
        yield conn
        conn.commit()
    finally:
        conn.close()

# ───────────────────────────────
# SAVE TRADE
# ───────────────────────────────
def save_trade(symbol, side, qty, price, pnl=0):
    with get_db() as conn:
        conn.execute("INSERT INTO trades (symbol, side, qty, price, pnl) VALUES (?,?,?,?,?)",
                     (symbol, side, qty, price, pnl))
    log.info(f"Trade saved: {symbol} {side} {qty}@{price} PnL={pnl}")

# ───────────────────────────────
# SAVE METRICS
# ───────────────────────────────
def save_metrics(capital, drawdown, trades_total, pnl):
    with get_db() as conn:
        conn.execute("INSERT INTO metrics (capital, drawdown, trades_total, pnl) VALUES (?,?,?,?)",
                     (capital, drawdown, trades_total, pnl))
    log.info(f"Metrics saved: Capital={capital} DD={drawdown:.2%} Trades={trades_total} PnL={pnl}")

# ───────────────────────────────
# EXPORT PROMETHEUS METRICS
# ───────────────────────────────
def prometheus_metrics():
    with get_db() as conn:
        row = conn.execute("SELECT * FROM metrics ORDER BY timestamp DESC LIMIT 1").fetchone()
        if not row:
            return Response("capital 0\ndrawdown 0\ntrades_total 0\npnl 0\n", mimetype="text/plain")

        metrics = f"""
capital {row['capital']}
drawdown {row['drawdown']}
trades_total {row['trades_total']}
pnl {row['pnl']}
"""
        return Response(metrics, mimetype="text/plain")
