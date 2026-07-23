import numpy as np
import pandas as pd

from backtest import Backtester


def _make_df(n=200, seed=7):
    rng = np.random.default_rng(seed)
    walk = 60000 + np.cumsum(rng.normal(0, 300, n))
    return pd.DataFrame({
        "timestamp": pd.date_range("2024-01-01", periods=n, freq="h"),
        "open": walk,
        "high": walk + 50,
        "low": walk - 50,
        "close": walk + rng.normal(0, 30, n),
        "volume": np.abs(rng.normal(1000, 200, n)),
    })


def test_backtest_runs_and_reports():
    bt = Backtester(capital=1000)
    bt.run(_make_df())
    s = bt.summary()
    assert s["round_trips"] == s["wins"] + s["losses"]
    assert "net_pnl" in s and "max_drawdown_pct" in s
    # Le capital final doit être cohérent avec le P&L net.
    assert abs((s["final_capital"] - 1000) - s["net_pnl"]) < 1e-6


def test_no_open_position_after_run():
    bt = Backtester(capital=1000)
    bt.run(_make_df())
    assert bt.position is None  # tout est clôturé en fin de série
