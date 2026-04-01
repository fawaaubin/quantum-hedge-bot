import pytest
import numpy as np
from strategies import Indicators, Strategies

def test_rsi_basic():
    prices = list(np.linspace(100, 120, 20))
    rsi_val = Indicators.rsi(prices)
    assert 0 <= rsi_val <= 100

def test_macd_basic():
    prices = list(np.linspace(100, 120, 30))
    macd_line, sig, hist = Indicators.macd(prices)
    assert macd_line is not None
    assert sig is not None
    assert hist is not None

def test_bollinger_basic():
    prices = list(np.linspace(100, 120, 25))
    upper, mid, lower = Indicators.bollinger(prices)
    assert upper > mid > lower

def test_aggregate_signals():
    prices = list(np.linspace(100, 120, 40))
    volumes = list(np.random.normal(1000, 200, 40))
    score, signals = Strategies.aggregate(prices, volumes)
    assert isinstance(score, int)
    assert isinstance(signals, dict)
