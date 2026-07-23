import pytest

from risk_manager import PortfolioRiskManager


def make_rm():
    return PortfolioRiskManager(1000, 0.5, 0.05, "off", 0.02)


def test_long_take_profit_realizes_gain():
    rm = make_rm()
    rm.add_position("BTCUSDT", 0.1, 100.0, "BUY", sl=98.0, tp=104.0)
    closed = rm.check_exits("BTCUSDT", 104.0)  # TP touché
    assert len(closed) == 1
    assert closed[0]["reason"] == "TP"
    assert closed[0]["pnl"] == pytest.approx((104 - 100) * 0.1)
    assert rm.capital == pytest.approx(1000 + 0.4)
    assert "BTCUSDT" not in rm.positions


def test_long_stop_loss_realizes_loss():
    rm = make_rm()
    rm.add_position("BTCUSDT", 0.1, 100.0, "BUY", sl=98.0, tp=104.0)
    closed = rm.check_exits("BTCUSDT", 97.0)  # SL touché
    assert closed[0]["reason"] == "SL"
    assert closed[0]["pnl"] == pytest.approx((97 - 100) * 0.1)
    assert rm.capital < 1000


def test_short_take_profit():
    rm = make_rm()
    rm.add_position("ETHUSDT", 1.0, 100.0, "SELL", sl=102.0, tp=96.0)
    closed = rm.check_exits("ETHUSDT", 96.0)  # TP short = prix baisse
    assert closed[0]["reason"] == "TP"
    assert closed[0]["pnl"] == pytest.approx(100 - 96)


def test_no_exit_when_price_between_bands():
    rm = make_rm()
    rm.add_position("BTCUSDT", 0.1, 100.0, "BUY", sl=98.0, tp=104.0)
    closed = rm.check_exits("BTCUSDT", 101.0)  # ni SL ni TP
    assert closed == []
    assert "BTCUSDT" in rm.positions


def test_drawdown_updates_after_losing_exit():
    rm = make_rm()
    rm.add_position("BTCUSDT", 1.0, 100.0, "BUY", sl=90.0, tp=120.0)
    rm.check_exits("BTCUSDT", 90.0)  # perte de 10
    assert rm.drawdown > 0
