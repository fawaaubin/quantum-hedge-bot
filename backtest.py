"""
backtest.py — Backtesting event-driven avec P&L réel.

Rédigé par l'expert trading + le dev senior.

Contrairement à la version initiale (qui comptait naïvement les BUY comme
"gains" et les SELL comme "pertes"), ce backtester :
- gère une position unique par symbole (long/flat/short),
- clôture la position sur signal inverse et calcule le P&L réalisé,
- applique des frais de transaction configurables,
- produit des métriques exploitables : P&L net, win rate, max drawdown,
  profit factor, nombre de trades.
"""

from __future__ import annotations

import logging

import numpy as np
import pandas as pd

from strategies import Strategies

log = logging.getLogger("QuantumHedge.Backtest")
logging.basicConfig(level=logging.INFO)


class Backtester:
    def __init__(self, capital: float = 1000, risk_pct: float = 0.02,
                 fee_rate: float = 0.001):
        self.capital = float(capital)
        self.initial_capital = float(capital)
        self.risk_pct = float(risk_pct)
        self.fee_rate = float(fee_rate)

        self.position = None          # dict {side, entry, qty} ou None
        self.trades = []              # trades exécutés (ouverture/clôture)
        self.closed = []              # P&L des round-trips clôturés
        self.equity_curve = []

    def _position_size(self, price: float, stop_loss: float) -> float:
        risk_amount = self.capital * self.risk_pct
        per_unit = abs(price - stop_loss) or (price * 0.01)
        return max(risk_amount / per_unit, 0.0)

    def _close(self, price: float, ts) -> None:
        if not self.position:
            return
        side = self.position["side"]
        entry = self.position["entry"]
        qty = self.position["qty"]
        direction = 1 if side == "BUY" else -1
        gross = (price - entry) * qty * direction
        fees = (entry + price) * qty * self.fee_rate
        pnl = gross - fees
        self.capital += pnl
        self.closed.append(pnl)
        self.trades.append({"time": ts, "action": "CLOSE", "side": side,
                            "price": price, "qty": qty, "pnl": pnl})
        self.position = None

    def _open(self, side: str, price: float, qty: float, ts) -> None:
        self.position = {"side": side, "entry": price, "qty": qty}
        self.trades.append({"time": ts, "action": "OPEN", "side": side,
                            "price": price, "qty": qty, "pnl": 0.0})

    def run(self, df: pd.DataFrame, symbol: str = "BTCUSDT"):
        prices, volumes = [], []
        for _, row in df.iterrows():
            close = float(row["close"])
            prices.append(close)
            volumes.append(float(row["volume"]))
            self.equity_curve.append(self.capital)

            if len(prices) < 30:
                continue

            score, _ = Strategies.aggregate(prices, volumes)
            ts = row.get("timestamp")

            if score > 2:
                if self.position and self.position["side"] == "SELL":
                    self._close(close, ts)
                if not self.position:
                    qty = self._position_size(close, close * 0.98)
                    self._open("BUY", close, qty, ts)
            elif score < -2:
                if self.position and self.position["side"] == "BUY":
                    self._close(close, ts)
                if not self.position:
                    qty = self._position_size(close, close * 1.02)
                    self._open("SELL", close, qty, ts)

        # Clôture mark-to-market en fin de série.
        if self.position and prices:
            self._close(prices[-1], df.iloc[-1].get("timestamp"))

        return self.trades

    def summary(self) -> dict:
        wins = [p for p in self.closed if p > 0]
        losses = [p for p in self.closed if p <= 0]
        gross_win = sum(wins)
        gross_loss = abs(sum(losses))
        equity = np.asarray(self.equity_curve, dtype=float)
        max_dd = 0.0
        if equity.size:
            peak = np.maximum.accumulate(equity)
            max_dd = float(np.max((peak - equity) / np.where(peak == 0, 1, peak)))

        result = {
            "round_trips": len(self.closed),
            "wins": len(wins),
            "losses": len(losses),
            "win_rate": round(len(wins) / len(self.closed), 4) if self.closed else 0.0,
            "net_pnl": round(self.capital - self.initial_capital, 2),
            "return_pct": round((self.capital / self.initial_capital - 1) * 100, 2),
            "profit_factor": round(gross_win / gross_loss, 3) if gross_loss else float("inf"),
            "max_drawdown_pct": round(max_dd * 100, 2),
            "final_capital": round(self.capital, 2),
        }
        log.info("Backtest summary: %s", result)
        return result


if __name__ == "__main__":
    # Exemple reproductible avec données fictives.
    rng = np.random.default_rng(42)
    n = 300
    walk = 60000 + np.cumsum(rng.normal(0, 300, n))
    data = {
        "timestamp": pd.date_range(start="2024-01-01", periods=n, freq="h"),
        "open": walk,
        "high": walk + rng.normal(50, 20, n),
        "low": walk - rng.normal(50, 20, n),
        "close": walk + rng.normal(0, 30, n),
        "volume": np.abs(rng.normal(1000, 200, n)),
    }
    df = pd.DataFrame(data)

    bt = Backtester(capital=1000)
    bt.run(df)
    print(bt.summary())
