"""
strategies.py — Moteur d'indicateurs & agrégation de signaux.

Rédigé par l'expert trading + le dev senior :
- Indicateurs classiques robustes (RSI, MACD, Bollinger, EMA, ATR).
- Un moteur d'agrégation multi-stratégies qui vote (-1 / 0 / +1) et
  renvoie un score entier signé + le détail des signaux (JSON-serializable).

Le score est volontairement un *entier* : chaque stratégie apporte une voix,
pondérée par sa conviction. app.py déclenche un achat si score > 2 et une
vente si score < -2 (consensus d'au moins ~3 stratégies).
"""

from __future__ import annotations

import logging
from typing import Dict, List, Tuple

import numpy as np

log = logging.getLogger("QuantumHedge.Strategies")

# Nombre minimal de points avant de produire un signal exploitable.
MIN_HISTORY = 30


class Indicators:
    """Indicateurs techniques purs (sans effet de bord)."""

    @staticmethod
    def _to_array(prices) -> np.ndarray:
        arr = np.asarray(prices, dtype=float)
        return arr[~np.isnan(arr)]

    @staticmethod
    def ema(prices, period: int) -> float:
        """Exponential Moving Average (dernière valeur)."""
        arr = Indicators._to_array(prices)
        if arr.size == 0:
            return float("nan")
        if arr.size < period:
            return float(arr.mean())
        alpha = 2.0 / (period + 1.0)
        ema = arr[0]
        for price in arr[1:]:
            ema = alpha * price + (1 - alpha) * ema
        return float(ema)

    @staticmethod
    def rsi(prices, period: int = 14) -> float:
        """Relative Strength Index (Wilder), borné [0, 100]."""
        arr = Indicators._to_array(prices)
        if arr.size < period + 1:
            return 50.0  # neutre tant qu'on n'a pas assez d'historique
        deltas = np.diff(arr)
        gains = np.where(deltas > 0, deltas, 0.0)
        losses = np.where(deltas < 0, -deltas, 0.0)
        avg_gain = gains[:period].mean()
        avg_loss = losses[:period].mean()
        # Lissage de Wilder
        for i in range(period, len(deltas)):
            avg_gain = (avg_gain * (period - 1) + gains[i]) / period
            avg_loss = (avg_loss * (period - 1) + losses[i]) / period
        if avg_loss == 0:
            return 100.0 if avg_gain > 0 else 50.0
        rs = avg_gain / avg_loss
        return float(100.0 - (100.0 / (1.0 + rs)))

    @staticmethod
    def macd(prices, fast: int = 12, slow: int = 26, signal: int = 9
             ) -> Tuple[float, float, float]:
        """MACD : renvoie (macd_line, signal_line, histogram) — dernières valeurs."""
        arr = Indicators._to_array(prices)
        if arr.size < 2:
            return 0.0, 0.0, 0.0

        def _ema_series(series: np.ndarray, period: int) -> np.ndarray:
            alpha = 2.0 / (period + 1.0)
            out = np.empty_like(series)
            out[0] = series[0]
            for i in range(1, series.size):
                out[i] = alpha * series[i] + (1 - alpha) * out[i - 1]
            return out

        ema_fast = _ema_series(arr, fast)
        ema_slow = _ema_series(arr, slow)
        macd_line = ema_fast - ema_slow
        signal_line = _ema_series(macd_line, signal)
        hist = macd_line - signal_line
        return float(macd_line[-1]), float(signal_line[-1]), float(hist[-1])

    @staticmethod
    def bollinger(prices, period: int = 20, num_std: float = 2.0
                  ) -> Tuple[float, float, float]:
        """Bandes de Bollinger : renvoie (upper, mid, lower)."""
        arr = Indicators._to_array(prices)
        if arr.size == 0:
            return 0.0, 0.0, 0.0
        window = arr[-period:] if arr.size >= period else arr
        mid = float(window.mean())
        std = float(window.std(ddof=0))
        return mid + num_std * std, mid, mid - num_std * std

    @staticmethod
    def atr_from_close(prices, period: int = 14) -> float:
        """Proxy de volatilité (ATR simplifié sur close) — utile au sizing."""
        arr = Indicators._to_array(prices)
        if arr.size < 2:
            return 0.0
        tr = np.abs(np.diff(arr))
        window = tr[-period:] if tr.size >= period else tr
        return float(window.mean())


class Strategies:
    """Agrégation multi-stratégies : chaque stratégie vote, on somme les voix."""

    @staticmethod
    def aggregate(prices: List[float], volumes: List[float]
                  ) -> Tuple[int, Dict[str, float]]:
        """
        Renvoie (score:int, signals:dict).

        score > 0 => biais acheteur, score < 0 => biais vendeur.
        signals contient le détail par stratégie (valeurs JSON-serializable).
        """
        arr = Indicators._to_array(prices)
        signals: Dict[str, float] = {}

        if arr.size < MIN_HISTORY:
            return 0, {"status": "warmup", "history": int(arr.size)}

        last = float(arr[-1])
        score = 0

        # 1) RSI — retour à la moyenne
        rsi = Indicators.rsi(arr)
        signals["rsi"] = round(rsi, 2)
        if rsi < 30:
            score += 1
        elif rsi > 70:
            score -= 1

        # 2) MACD — momentum
        macd_line, macd_signal, hist = Indicators.macd(arr)
        signals["macd_hist"] = round(hist, 4)
        if hist > 0 and macd_line > macd_signal:
            score += 1
        elif hist < 0 and macd_line < macd_signal:
            score -= 1

        # 3) Bollinger — retour à la moyenne aux extrêmes
        upper, mid, lower = Indicators.bollinger(arr)
        signals["bollinger_pos"] = round((last - mid) / (upper - mid), 3) if upper != mid else 0.0
        if last <= lower:
            score += 1
        elif last >= upper:
            score -= 1

        # 4) EMA cross — tendance
        ema_fast = Indicators.ema(arr, 12)
        ema_slow = Indicators.ema(arr, 26)
        signals["ema_trend"] = 1 if ema_fast > ema_slow else -1
        score += 1 if ema_fast > ema_slow else -1

        # 5) Volume surge — confirmation (amplifie le signal dominant)
        vol = Indicators._to_array(volumes)
        if vol.size >= 20:
            recent = vol[-1]
            base = vol[-20:-1]
            surge = bool(recent > (base.mean() + 2 * base.std(ddof=0)))
            signals["volume_surge"] = surge
            if surge and score != 0:
                score += 1 if score > 0 else -1

        signals["score"] = int(score)
        return int(score), signals
