import os

# ───────────────────────────────
# API KEYS & SECRETS
# ───────────────────────────────
BINANCE_API_KEY     = os.getenv("BINANCE_API_KEY", "")
BINANCE_API_SECRET  = os.getenv("BINANCE_API_SECRET", "")
FLASK_SECRET        = os.getenv("FLASK_SECRET", "supersecret")

# ───────────────────────────────
# TRADING PARAMETERS
# ───────────────────────────────
CAPITAL             = float(os.getenv("CAPITAL", 1000))       # capital initial
RISK_PCT            = float(os.getenv("RISK_PCT", 0.02))      # % du capital risqué par trade
MAX_DRAWDOWN        = float(os.getenv("MAX_DRAWDOWN", 0.10))  # drawdown max toléré
MAX_PER_PAIR_RISK   = float(os.getenv("MAX_PER_PAIR_RISK", 0.05)) # risque max par paire
HEDGE_MODE          = os.getenv("HEDGE_MODE", "auto")         # "on", "off", "auto"

# ───────────────────────────────
# SYMBOLS (multi‑paires)
# ───────────────────────────────
SYMBOLS             = os.getenv("SYMBOLS", "BTCUSDT,ETHUSDT,BNBUSDT").split(",")

# ───────────────────────────────
# TESTNET / MAINNET
# ───────────────────────────────
TESTNET_SPOT        = os.getenv("TESTNET_SPOT", "true").lower() == "true"
TESTNET_FUTURES     = os.getenv("TESTNET_FUTURES", "true").lower() == "true"

# ───────────────────────────────
# ALERTS CONFIG
# ───────────────────────────────
TELEGRAM_TOKEN      = os.getenv("TELEGRAM_TOKEN", "")
TELEGRAM_CHAT_ID    = os.getenv("TELEGRAM_CHAT_ID", "")
SLACK_WEBHOOK       = os.getenv("SLACK_WEBHOOK", "")
EMAIL_CONFIG        = {
    "smtp": os.getenv("EMAIL_SMTP", ""),
    "port": int(os.getenv("EMAIL_PORT", "587")),
    "user": os.getenv("EMAIL_USER", ""),
    "password": os.getenv("EMAIL_PASSWORD", ""),
    "from": os.getenv("EMAIL_FROM", ""),
    "to": os.getenv("EMAIL_TO", "")
} if os.getenv("EMAIL_SMTP") else None

# ───────────────────────────────
# MONITORING
# ───────────────────────────────
PROMETHEUS_ENABLED  = os.getenv("PROMETHEUS_ENABLED", "true").lower() == "true"
GRAFANA_ENABLED     = os.getenv("GRAFANA_ENABLED", "true").lower() == "true"
