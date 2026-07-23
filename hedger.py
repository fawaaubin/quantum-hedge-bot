"""
hedger.py — Exécution des ordres Spot/Futures + couverture automatique.

Rédigé par l'expert trading + le dev senior.

Conception :
- `spot_client` / `futures_client` sont injectés (ex : client Binance).
  S'ils valent None, on est en **mode simulation** : l'ordre est journalisé
  et persisté, mais aucun appel réseau n'est fait. C'est le mode par défaut
  pour le backtest, les tests et le paper-trading.
- `auto_hedge()` ouvre une jambe Futures inverse pour neutraliser le risque
  directionnel quand `hedge_mode` vaut "on" ou "auto".

La persistance en base est *best-effort* : une erreur d'écriture ne doit
jamais bloquer ni corrompre l'exécution (le statut de l'ordre reste fiable).
"""

from __future__ import annotations

import logging
import time
from typing import Optional

log = logging.getLogger("QuantumHedge.Hedger")


class FuturesHedger:
    def __init__(self, spot_client, futures_client, risk_manager,
                 symbol_spot: str, symbol_futures: str):
        self.spot_client = spot_client
        self.futures_client = futures_client
        self.rm = risk_manager
        self.symbol_spot = symbol_spot
        self.symbol_futures = symbol_futures

    # ───────────────────────────────
    # PERSISTENCE (best-effort)
    # ───────────────────────────────
    @staticmethod
    def _persist(symbol, side, qty, price, pnl=0.0):
        try:
            from database import save_trade
            save_trade(symbol, side, qty, price, pnl)
        except Exception as e:  # pragma: no cover - la persistance ne doit pas bloquer
            log.debug("Persistance ignorée (%s)", e)

    # ───────────────────────────────
    # SPOT
    # ───────────────────────────────
    def execute_spot_order(self, side: str, qty: float, price: float,
                           strategy: str = "manual",
                           sl: Optional[float] = None,
                           tp: Optional[float] = None) -> dict:
        order = {
            "market": "spot",
            "symbol": self.symbol_spot,
            "side": side,
            "qty": float(qty),
            "price": float(price),
            "sl": sl,
            "tp": tp,
            "strategy": strategy,
            "ts": time.time(),
        }

        if self.spot_client is None:
            order["status"] = "simulated"
            log.info("[SIM] SPOT %s %s %s @ %s (%s)", side, qty, self.symbol_spot, price, strategy)
        else:
            try:
                resp = self.spot_client.create_order(
                    symbol=self.symbol_spot, side=side,
                    type="MARKET", quantity=qty)
                order["status"] = "filled"
                order["exchange"] = resp
                log.info("SPOT %s %s %s @ %s -> filled", side, qty, self.symbol_spot, price)
            except Exception as e:
                order["status"] = "error"
                order["error"] = str(e)
                log.error("Échec ordre spot: %s", e)
                return order

        self._persist(self.symbol_spot, side, qty, price)
        return order

    # ───────────────────────────────
    # FUTURES
    # ───────────────────────────────
    def execute_futures_order(self, side: str, qty: float, price: float,
                              reduce_only: bool = False) -> dict:
        order = {
            "market": "futures",
            "symbol": self.symbol_futures,
            "side": side,
            "qty": float(qty),
            "price": float(price),
            "reduce_only": reduce_only,
            "ts": time.time(),
        }

        if self.futures_client is None:
            order["status"] = "simulated"
            log.info("[SIM] FUTURES %s %s %s @ %s", side, qty, self.symbol_futures, price)
        else:
            try:
                resp = self.futures_client.create_order(
                    symbol=self.symbol_futures, side=side,
                    type="MARKET", quantity=qty, reduceOnly=reduce_only)
                order["status"] = "filled"
                order["exchange"] = resp
                log.info("FUTURES %s %s %s @ %s -> filled", side, qty, self.symbol_futures, price)
            except Exception as e:
                order["status"] = "error"
                order["error"] = str(e)
                log.error("Échec ordre futures: %s", e)
                return order

        self._persist(self.symbol_futures, f"HEDGE_{side}", qty, price)
        return order

    # ───────────────────────────────
    # AUTO-HEDGE
    # ───────────────────────────────
    def auto_hedge(self, spot_qty: float, price: float,
                   spot_side: str = "BUY", ratio: float = 1.0) -> Optional[dict]:
        """
        Ouvre une jambe Futures inverse pour couvrir la position spot.
        `ratio` = fraction couverte (1.0 = couverture totale, 0.5 = 50%).
        """
        mode = getattr(self.rm, "hedge_mode", "off")
        if mode not in ("on", "auto"):
            return None

        hedge_side = "SELL" if spot_side.upper() == "BUY" else "BUY"
        hedge_qty = round(float(spot_qty) * float(ratio), 8)
        if hedge_qty <= 0:
            return None

        log.info("Auto-hedge %s %s (couverture de %s spot)", hedge_side, hedge_qty, spot_qty)
        return self.execute_futures_order(hedge_side, hedge_qty, price, reduce_only=False)
