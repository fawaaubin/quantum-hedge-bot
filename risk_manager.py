"""
risk_manager.py — Gestion du risque portefeuille (niveau edge).

Rédigé par l'expert trading + le dev senior. Principes appliqués :
- Position sizing basé sur le risque : on ne risque jamais plus de
  `risk_pct` du capital par trade (distance au stop-loss).
- Plafond d'exposition par paire (`max_per_pair_risk`).
- Suivi du drawdown à partir du pic de capital (high-water mark).
- Coupe-circuit : `can_trade()` interdit toute nouvelle prise de position
  au-delà du drawdown maximal toléré.

Aucune dépendance externe : thread-safe via un verrou pour la boucle
multi-paires + les requêtes Flask concurrentes.
"""

from __future__ import annotations

import logging
import threading
from typing import Dict, List

log = logging.getLogger("QuantumHedge.Risk")


class PortfolioRiskManager:
    def __init__(self, capital: float, max_drawdown: float,
                 max_per_pair_risk: float, hedge_mode: str, risk_pct: float):
        self.initial_capital = float(capital)
        self.capital = float(capital)
        self.peak_capital = float(capital)
        self.max_drawdown = float(max_drawdown)
        self.max_per_pair_risk = float(max_per_pair_risk)
        self.hedge_mode = hedge_mode
        self.risk_pct = float(risk_pct)

        self.drawdown = 0.0
        self.realized_pnl = 0.0
        self.positions: Dict[str, List[dict]] = {}
        self._lock = threading.RLock()

    # ───────────────────────────────
    # POSITION SIZING
    # ───────────────────────────────
    def calc_position_size(self, symbol: str, price: float, stop_loss: float) -> float:
        """
        Taille de position telle que la perte au stop = risk_pct * capital,
        plafonnée par l'exposition notionnelle max autorisée par paire.
        """
        with self._lock:
            price = float(price)
            if price <= 0:
                return 0.0

            risk_amount = self.capital * self.risk_pct
            per_unit_risk = abs(price - float(stop_loss))
            # Garde-fou si SL absent/incohérent : on retombe sur 1% du prix.
            if per_unit_risk <= 0:
                per_unit_risk = price * 0.01

            qty = risk_amount / per_unit_risk

            # Plafond d'exposition notionnelle par paire.
            max_notional = self.capital * self.max_per_pair_risk / max(self.risk_pct, 1e-9)
            max_qty = max_notional / price
            qty = min(qty, max_qty)

            return round(max(qty, 0.0), 8)

    # ───────────────────────────────
    # POSITIONS
    # ───────────────────────────────
    def add_position(self, symbol: str, qty: float, price: float, side: str,
                     sl: float = None, tp: float = None) -> None:
        with self._lock:
            self.positions.setdefault(symbol, []).append({
                "qty": float(qty),
                "entry": float(price),
                "side": side,
                "sl": float(sl) if sl is not None else None,
                "tp": float(tp) if tp is not None else None,
            })
            log.info("Position ouverte %s %s qty=%s @ %s (SL=%s TP=%s)",
                     symbol, side, qty, price, sl, tp)

    @staticmethod
    def _realized_pnl(leg: dict, exit_price: float) -> float:
        direction = 1 if leg["side"] == "BUY" else -1
        return (exit_price - leg["entry"]) * leg["qty"] * direction

    def check_exits(self, symbol: str, price: float) -> list:
        """
        Évalue les positions ouvertes sur `symbol` au prix courant et clôture
        celles dont le stop-loss ou le take-profit est touché. Le P&L réalisé
        est appliqué au capital. Renvoie la liste des clôtures.
        """
        price = float(price)
        closed = []
        with self._lock:
            legs = self.positions.get(symbol, [])
            survivors = []
            for leg in legs:
                sl, tp = leg.get("sl"), leg.get("tp")
                hit = None
                if leg["side"] == "BUY":
                    if sl is not None and price <= sl:
                        hit = "SL"
                    elif tp is not None and price >= tp:
                        hit = "TP"
                else:  # SELL / short
                    if sl is not None and price >= sl:
                        hit = "SL"
                    elif tp is not None and price <= tp:
                        hit = "TP"

                if hit:
                    pnl = self._realized_pnl(leg, price)
                    closed.append({
                        "symbol": symbol, "side": leg["side"], "qty": leg["qty"],
                        "entry": leg["entry"], "exit": price, "reason": hit, "pnl": pnl,
                    })
                else:
                    survivors.append(leg)

            if survivors:
                self.positions[symbol] = survivors
            else:
                self.positions.pop(symbol, None)

        # update_pnl prend son propre verrou : on l'appelle hors section critique.
        for c in closed:
            self.update_pnl(c["pnl"])
        return closed

    def close_position(self, symbol: str) -> None:
        with self._lock:
            self.positions.pop(symbol, None)

    def total_exposure(self) -> float:
        with self._lock:
            return sum(p["qty"] * p["entry"]
                       for legs in self.positions.values() for p in legs)

    # ───────────────────────────────
    # PNL & DRAWDOWN
    # ───────────────────────────────
    def update_pnl(self, delta: float) -> None:
        """Applique un P&L réalisé et met à jour le high-water mark / drawdown."""
        with self._lock:
            delta = float(delta)
            self.capital += delta
            self.realized_pnl += delta
            if self.capital > self.peak_capital:
                self.peak_capital = self.capital
            if self.peak_capital > 0:
                self.drawdown = max(0.0, (self.peak_capital - self.capital) / self.peak_capital)
            log.info("PnL %+.2f | capital=%.2f | drawdown=%.2f%%",
                     delta, self.capital, self.drawdown * 100)

    # ───────────────────────────────
    # COUPE-CIRCUIT
    # ───────────────────────────────
    def can_trade(self) -> bool:
        """False si le drawdown max est atteint — plus aucune nouvelle position."""
        with self._lock:
            return self.drawdown < self.max_drawdown

    def snapshot(self) -> dict:
        with self._lock:
            return {
                "capital": round(self.capital, 2),
                "initial_capital": self.initial_capital,
                "peak_capital": round(self.peak_capital, 2),
                "drawdown": round(self.drawdown, 4),
                "realized_pnl": round(self.realized_pnl, 2),
                "open_positions": {k: list(v) for k, v in self.positions.items()},
                "can_trade": self.drawdown < self.max_drawdown,
            }
