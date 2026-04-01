import logging
import pandas as pd
import numpy as np
from strategies import Strategies
from risk_manager import PortfolioRiskManager

log = logging.getLogger("QuantumHedgeBacktest")
logging.basicConfig(level=logging.INFO)

class Backtester:
    def __init__(self, capital=1000, risk_pct=0.02):
        self.rm = PortfolioRiskManager(capital, 0.2, 0.05, "off", risk_pct)
        self.trades = []

    def run(self, df, symbol="BTCUSDT"):
        """
        df doit contenir les colonnes: ['timestamp','open','high','low','close','volume']
        """
        prices = []
        volumes = []
        for i, row in df.iterrows():
            prices.append(row["close"])
            volumes.append(row["volume"])

            if len(prices) < 30: 
                continue

            score, signals = Strategies.aggregate(prices, volumes)

            if score > 2:
                sl = row["close"] * 0.98
                tp = row["close"] * 1.04
                qty = self.rm.calc_position_size(symbol, row["close"], sl)
                self.trades.append({"time":row["timestamp"],"symbol":symbol,"side":"BUY","price":row["close"],"qty":qty})
                self.rm.add_position(symbol, qty, row["close"], "BUY")

            elif score < -2:
                sl = row["close"] * 1.02
                tp = row["close"] * 0.96
                qty = self.rm.calc_position_size(symbol, row["close"], sl)
                self.trades.append({"time":row["timestamp"],"symbol":symbol,"side":"SELL","price":row["close"],"qty":qty})
                self.rm.add_position(symbol, qty, row["close"], "SELL")

        return self.trades

    def summary(self):
        wins = sum(1 for t in self.trades if t["side"]=="BUY")
        losses = sum(1 for t in self.trades if t["side"]=="SELL")
        log.info(f"Total trades={len(self.trades)} | Wins={wins} | Losses={losses}")
        return {"total":len(self.trades),"wins":wins,"losses":losses,"capital":self.rm.capital}


if __name__ == "__main__":
    # Exemple avec données fictives
    data = {
        "timestamp": pd.date_range(start="2024-01-01", periods=200, freq="D"),
        "open": np.random.normal(60000,500,200),
        "high": np.random.normal(60500,500,200),
        "low": np.random.normal(59500,500,200),
        "close": np.random.normal(60000,500,200),
        "volume": np.random.normal(1000,200,200)
    }
    df = pd.DataFrame(data)

    bt = Backtester(capital=1000)
    trades = bt.run(df)
    summary = bt.summary()
    print(summary)
