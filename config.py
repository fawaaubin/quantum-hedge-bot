import os
import secrets
import logging

log = logging.getLogger("QuantumHedge.Config")

# ───────────────────────────────
# API KEYS & SECRETS
# ───────────────────────────────
BINANCE_API_KEY     = os.getenv("BINANCE_API_KEY", "")
BINANCE_API_SECRET  = os.getenv("BINANCE_API_SECRET", "")

# Sécurité : plus de secret par défaut prévisible ("supersecret").
# En prod on EXIGE un FLASK_SECRET fourni ; sinon on en génère un éphémère.
FLASK_SECRET = os.getenv("FLASK_SECRET")
if not FLASK_SECRET:
    FLASK_SECRET = secrets.token_hex(32)
    log.warning("FLASK_SECRET non défini : génération d'un secret éphémère "
                "(les sessions seront invalidées à chaque redémarrage).")

# Jeton d'API pour protéger les endpoints sensibles (/manual_trade).
# Sans jeton défini, l'endpoint d'ordre manuel est refusé par défaut.
API_TOKEN = os.getenv("API_TOKEN", "")

# ───────────────────────────────
# TRADING PARAMETERS
# ───────────────────────────────
CAPITAL             = float(os.getenv("CAPITAL", 1000))       # capital initial
RISK_PCT            = float(os.getenv("RISK_PCT", 0.02))      # % du capital risqué par trade
MAX_DRAWDOWN        = float(os.getenv("MAX_DRAWDOWN", 0.10))  # drawdown max toléré
MAX_PER_PAIR_RISK   = float(os.getenv("MAX_PER_PAIR_RISK", 0.05))  # risque max par paire
HEDGE_MODE          = os.getenv("HEDGE_MODE", "auto")        # "on", "off", "auto"

# ───────────────────────────────
# SYMBOLS (multi‑paires)
# ───────────────────────────────
SYMBOLS             = os.getenv("SYMBOLS", "BTCUSDT,ETHUSDT,BNBUSDT").split(",")
SYMBOL_SPOT         = os.getenv("SYMBOL_SPOT", SYMBOLS[0] if SYMBOLS else "BTCUSDT")
SYMBOL_FUTURES      = os.getenv("SYMBOL_FUTURES", SYMBOL_SPOT)

# ───────────────────────────────
# DATA FEED
# ───────────────────────────────
# "sim"  -> prix simulés (paper-trading / démo, aucun réseau)
# "binance" -> flux prix réels via l'API publique Binance
DATA_FEED           = os.getenv("DATA_FEED", "sim").lower()
POLL_INTERVAL       = float(os.getenv("POLL_INTERVAL", "5"))  # secondes entre deux cycles

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
# WEB / CORS
# ───────────────────────────────
# Origines autorisées pour SocketIO/CORS. "*" est déconseillé en prod.
CORS_ORIGINS        = os.getenv("CORS_ORIGINS", "*")

# ───────────────────────────────
# MONITORING
# ───────────────────────────────
PROMETHEUS_ENABLED  = os.getenv("PROMETHEUS_ENABLED", "true").lower() == "true"
GRAFANA_ENABLED     = os.getenv("GRAFANA_ENABLED", "true").lower() == "true"
