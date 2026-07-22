// Package config charge et valide la configuration YAML du bot.
// Les secrets (clés API, token Telegram) ne vivent JAMAIS dans le YAML :
// ils sont chargés exclusivement depuis les variables d'environnement.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config est la configuration racine du bot.
type Config struct {
	Binance BinanceConfig `yaml:"binance"`
	Trading TradingConfig `yaml:"trading"`
	Risk    RiskConfig    `yaml:"risk"`
	Gateway GatewayConfig `yaml:"gateway"`
	Store   StoreConfig   `yaml:"store"`
	Monitor MonitorConfig `yaml:"monitor"`
	Logging LoggingConfig `yaml:"logging"`
}

// BinanceConfig regroupe les endpoints et les paramètres d'API.
// Les clés API sont injectées depuis l'environnement, pas depuis le YAML.
type BinanceConfig struct {
	RESTBaseURL string        `yaml:"rest_base_url"`
	WSBaseURL   string        `yaml:"ws_base_url"`
	RecvWindow  time.Duration `yaml:"recv_window"`
	Testnet     bool          `yaml:"testnet"`

	// Rate limiting côté client (token bucket).
	RateLimitCapacity  float64 `yaml:"rate_limit_capacity"`       // jetons max
	RateLimitRefillPer float64 `yaml:"rate_limit_refill_per_sec"` // jetons/s

	// Chargés depuis BINANCE_API_KEY / BINANCE_API_SECRET, jamais loggés.
	APIKey    string `yaml:"-"`
	APISecret string `yaml:"-"`
}

// TradingConfig limite le périmètre de trading.
type TradingConfig struct {
	Symbol string `yaml:"symbol"`
	// ReplaceThresholdPct est la dérive de prix (fraction, ex 0.0001 =
	// 0,01 %) au-delà de laquelle un ordre d'entrée non exécuté est
	// annulé et replacé au nouveau meilleur prix.
	ReplaceThresholdPct float64 `yaml:"replace_threshold_pct"`
}

// RiskConfig porte les paramètres du risk manager (Phase 4).
type RiskConfig struct {
	InitialCapital    float64       `yaml:"initial_capital"`     // capital de référence (quote)
	MaxRiskPerTrade   float64       `yaml:"max_risk_per_trade"`  // ex: 0.01 = 1 %
	MaxAbsoluteRisk   float64       `yaml:"max_absolute_risk"`   // ex: 0.02 = 2 %
	MaxOpenPositions  int           `yaml:"max_open_positions"`  // ex: 1
	BreakerLookback   int           `yaml:"breaker_lookback"`    // ex: 10 trades
	BreakerLossPct    float64       `yaml:"breaker_loss_pct"`    // ex: 0.05 = 5 %
	BreakerCooldown   time.Duration `yaml:"breaker_cooldown"`    // durée de suspension
	StopLossPct       float64       `yaml:"stop_loss_pct"`       // ex: 0.01
	TakeProfitPct     float64       `yaml:"take_profit_pct"`     // ex: 0.02
	TrailingActivePct float64       `yaml:"trailing_active_pct"` // ex: 0.01
	TrailingStepPct   float64       `yaml:"trailing_step_pct"`   // ex: 0.005
}

// GatewayConfig porte les paramètres de résilience WebSocket (Phase 2).
type GatewayConfig struct {
	EventBufferSize  int           `yaml:"event_buffer_size"` // ex: 256
	BackoffInitial   time.Duration `yaml:"backoff_initial"`
	BackoffMax       time.Duration `yaml:"backoff_max"`
	SnapshotTimeout  time.Duration `yaml:"snapshot_timeout"`
	SnapshotDepth    int           `yaml:"snapshot_depth"`     // ex: 1000
	DepthStreamSpeed string        `yaml:"depth_stream_speed"` // ex: "100ms"
}

// StoreConfig porte les paramètres de persistance SQLite (Phase 6).
type StoreConfig struct {
	Path string `yaml:"path"`
}

// MonitorConfig porte les paramètres d'observabilité (Phase 6).
type MonitorConfig struct {
	ListenAddr string `yaml:"listen_addr"`

	// Chargés depuis TELEGRAM_TOKEN / TELEGRAM_CHAT_ID, jamais loggés.
	TelegramToken  string `yaml:"-"`
	TelegramChatID string `yaml:"-"`
}

// LoggingConfig configure le logger structuré.
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json ou text
}

// Load lit le fichier YAML, applique les valeurs par défaut, injecte les
// secrets depuis l'environnement puis valide l'ensemble.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lecture config %s: %w", path, err)
	}

	cfg := defaults()
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse YAML %s: %w", path, err)
	}

	cfg.Binance.APIKey = os.Getenv("BINANCE_API_KEY")
	cfg.Binance.APISecret = os.Getenv("BINANCE_API_SECRET")
	cfg.Monitor.TelegramToken = os.Getenv("TELEGRAM_TOKEN")
	cfg.Monitor.TelegramChatID = os.Getenv("TELEGRAM_CHAT_ID")

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// defaults retourne une configuration pré-remplie avec les valeurs du
// cahier des charges (testnet, BTCUSDT, paramètres de risque initiaux).
func defaults() *Config {
	return &Config{
		Binance: BinanceConfig{
			RESTBaseURL:        "https://testnet.binance.vision",
			WSBaseURL:          "wss://stream.testnet.binance.vision",
			RecvWindow:         5000 * time.Millisecond,
			Testnet:            true,
			RateLimitCapacity:  10,
			RateLimitRefillPer: 5,
		},
		Trading: TradingConfig{Symbol: "BTCUSDT", ReplaceThresholdPct: 0.0001},
		Risk: RiskConfig{
			InitialCapital:    1000,
			MaxRiskPerTrade:   0.01,
			MaxAbsoluteRisk:   0.02,
			MaxOpenPositions:  1,
			BreakerLookback:   10,
			BreakerLossPct:    0.05,
			BreakerCooldown:   4 * time.Hour,
			StopLossPct:       0.01,
			TakeProfitPct:     0.02,
			TrailingActivePct: 0.01,
			TrailingStepPct:   0.005,
		},
		Gateway: GatewayConfig{
			EventBufferSize:  256,
			BackoffInitial:   500 * time.Millisecond,
			BackoffMax:       30 * time.Second,
			SnapshotTimeout:  5 * time.Second,
			SnapshotDepth:    1000,
			DepthStreamSpeed: "100ms",
		},
		Store:   StoreConfig{Path: "data/bot.db"},
		Monitor: MonitorConfig{ListenAddr: ":9090"},
		Logging: LoggingConfig{Level: "info", Format: "json"},
	}
}

// Validate vérifie les bornes et la cohérence de la configuration.
// Les clés API ne sont pas exigées en Phase 1 (aucun appel signé).
func (c *Config) Validate() error {
	var errs []error

	if c.Trading.Symbol == "" {
		errs = append(errs, errors.New("trading.symbol est requis"))
	}
	if c.Binance.RESTBaseURL == "" || c.Binance.WSBaseURL == "" {
		errs = append(errs, errors.New("binance.rest_base_url et binance.ws_base_url sont requis"))
	}
	if !c.Binance.Testnet {
		errs = append(errs, errors.New("binance.testnet doit être true (version initiale limitée au Spot Testnet)"))
	}
	if c.Binance.RecvWindow <= 0 || c.Binance.RecvWindow > 60*time.Second {
		errs = append(errs, errors.New("binance.recv_window doit être dans (0, 60s]"))
	}
	if c.Binance.RateLimitCapacity < 1 || c.Binance.RateLimitRefillPer <= 0 {
		errs = append(errs, errors.New("binance.rate_limit_capacity et rate_limit_refill_per_sec doivent être positifs"))
	}
	if c.Trading.ReplaceThresholdPct <= 0 || c.Trading.ReplaceThresholdPct > 0.01 {
		errs = append(errs, errors.New("trading.replace_threshold_pct doit être dans (0, 0.01]"))
	}
	if c.Risk.MaxRiskPerTrade <= 0 || c.Risk.MaxRiskPerTrade > 1 {
		errs = append(errs, errors.New("risk.max_risk_per_trade doit être dans (0, 1]"))
	}
	if c.Risk.MaxAbsoluteRisk < c.Risk.MaxRiskPerTrade || c.Risk.MaxAbsoluteRisk > 1 {
		errs = append(errs, errors.New("risk.max_absolute_risk doit être >= max_risk_per_trade et <= 1"))
	}
	if c.Risk.MaxOpenPositions < 1 {
		errs = append(errs, errors.New("risk.max_open_positions doit être >= 1"))
	}
	if c.Risk.BreakerLookback < 1 || c.Risk.BreakerLossPct <= 0 {
		errs = append(errs, errors.New("risk.breaker_lookback et risk.breaker_loss_pct doivent être positifs"))
	}
	if c.Risk.InitialCapital <= 0 {
		errs = append(errs, errors.New("risk.initial_capital doit être positif"))
	}
	if c.Risk.BreakerCooldown <= 0 {
		errs = append(errs, errors.New("risk.breaker_cooldown doit être positif"))
	}
	if c.Risk.StopLossPct <= 0 || c.Risk.TakeProfitPct <= 0 ||
		c.Risk.TrailingActivePct <= 0 || c.Risk.TrailingStepPct <= 0 {
		errs = append(errs, errors.New("risk.stop_loss_pct, take_profit_pct, trailing_active_pct et trailing_step_pct doivent être positifs"))
	}
	if c.Gateway.EventBufferSize < 1 {
		errs = append(errs, errors.New("gateway.event_buffer_size doit être >= 1"))
	}
	if c.Gateway.BackoffInitial <= 0 || c.Gateway.BackoffMax < c.Gateway.BackoffInitial {
		errs = append(errs, errors.New("gateway.backoff_initial et gateway.backoff_max sont incohérents"))
	}
	if c.Gateway.SnapshotDepth < 5 || c.Gateway.SnapshotDepth > 5000 {
		errs = append(errs, errors.New("gateway.snapshot_depth doit être dans [5, 5000]"))
	}
	if c.Gateway.SnapshotTimeout <= 0 {
		errs = append(errs, errors.New("gateway.snapshot_timeout doit être positif"))
	}
	if c.Store.Path == "" {
		errs = append(errs, errors.New("store.path est requis"))
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("logging.level invalide: %q", c.Logging.Level))
	}
	switch c.Logging.Format {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("logging.format invalide: %q", c.Logging.Format))
	}

	return errors.Join(errs...)
}
