import pytest
from risk_manager import PortfolioRiskManager

def test_position_sizing():
    rm = PortfolioRiskManager(1000, 0.2, 0.05, "off", 0.02)
    qty = rm.calc_position_size("BTCUSDT", 100, 95)
    assert qty > 0

def test_update_pnl_and_drawdown():
    rm = PortfolioRiskManager(1000, 0.2, 0.05, "off", 0.02)
    rm.update_pnl(-100)
    assert rm.capital == 900
    assert rm.drawdown > 0
