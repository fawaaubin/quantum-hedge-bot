import pytest
from hedger import FuturesHedger
from risk_manager import PortfolioRiskManager

def test_spot_order_simulation():
    rm = PortfolioRiskManager(1000, 0.2, 0.05, "off", 0.02)
    hedger = FuturesHedger(None, None, rm, "BTCUSDT", "BTCUSDT")
    result = hedger.execute_spot_order("BUY", 0.1, 60000)
    assert result["status"] == "simulated"

def test_futures_order_simulation():
    rm = PortfolioRiskManager(1000, 0.2, 0.05, "off", 0.02)
    hedger = FuturesHedger(None, None, rm, "BTCUSDT", "BTCUSDT")
    result = hedger.execute_futures_order("SELL", 0.1, 60000)
    assert result["status"] == "simulated"
