import pytest

from risk_manager import PortfolioRiskManager


def make_rm():
    return PortfolioRiskManager(1000, 0.10, 0.05, "off", 0.02)


def test_position_sizing_positive():
    rm = make_rm()
    qty = rm.calc_position_size("BTCUSDT", 100, 95)
    assert qty > 0
    # On risque 2% de 1000 = 20 sur une distance de 5 => qty ~ 4 (plafond inclus)
    assert qty <= 4 + 1e-6


def test_position_sizing_handles_zero_distance():
    rm = make_rm()
    qty = rm.calc_position_size("BTCUSDT", 100, 100)  # SL == prix
    assert qty > 0  # ne doit pas diviser par zéro


def test_update_pnl_and_drawdown():
    rm = make_rm()
    rm.update_pnl(-100)
    assert rm.capital == 900
    assert rm.drawdown == pytest.approx(0.1)


def test_high_water_mark():
    rm = make_rm()
    rm.update_pnl(200)          # capital 1200, nouveau pic
    rm.update_pnl(-120)         # capital 1080
    assert rm.peak_capital == 1200
    assert rm.drawdown == pytest.approx((1200 - 1080) / 1200)


def test_circuit_breaker():
    rm = make_rm()
    assert rm.can_trade() is True
    rm.update_pnl(-150)         # drawdown 15% > 10% max
    assert rm.can_trade() is False


def test_snapshot_is_json_friendly():
    rm = make_rm()
    rm.add_position("BTCUSDT", 0.1, 60000, "BUY")
    snap = rm.snapshot()
    assert snap["open_positions"]["BTCUSDT"][0]["qty"] == 0.1
    assert "capital" in snap and "drawdown" in snap
