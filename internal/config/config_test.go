package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	path := writeConfig(t, "trading:\n  symbol: BTCUSDT\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Trading.Symbol != "BTCUSDT" {
		t.Errorf("symbol = %q, attendu BTCUSDT", cfg.Trading.Symbol)
	}
	if !cfg.Binance.Testnet {
		t.Error("testnet doit être true par défaut")
	}
	if cfg.Binance.RecvWindow != 5000*time.Millisecond {
		t.Errorf("recv_window = %v, attendu 5s", cfg.Binance.RecvWindow)
	}
	if cfg.Risk.MaxRiskPerTrade != 0.01 || cfg.Risk.MaxAbsoluteRisk != 0.02 {
		t.Errorf("paramètres de risque par défaut incorrects: %+v", cfg.Risk)
	}
	if cfg.Gateway.EventBufferSize != 256 {
		t.Errorf("event_buffer_size = %d, attendu 256", cfg.Gateway.EventBufferSize)
	}
}

func TestLoadSecretsFromEnv(t *testing.T) {
	t.Setenv("BINANCE_API_KEY", "test-key")
	t.Setenv("BINANCE_API_SECRET", "test-secret")

	path := writeConfig(t, "trading:\n  symbol: BTCUSDT\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Binance.APIKey != "test-key" || cfg.Binance.APISecret != "test-secret" {
		t.Error("les clés API doivent être chargées depuis l'environnement")
	}
}

func TestValidateRejectsMainnet(t *testing.T) {
	path := writeConfig(t, "binance:\n  testnet: false\n")
	if _, err := Load(path); err == nil {
		t.Fatal("une config mainnet doit être rejetée en version initiale")
	}
}

func TestValidateRejectsBadRisk(t *testing.T) {
	path := writeConfig(t, "risk:\n  max_risk_per_trade: 0\n")
	if _, err := Load(path); err == nil {
		t.Fatal("max_risk_per_trade = 0 doit être rejeté")
	}
}

func TestValidateRejectsBadLogging(t *testing.T) {
	path := writeConfig(t, "logging:\n  level: verbose\n")
	if _, err := Load(path); err == nil {
		t.Fatal("un niveau de log inconnu doit être rejeté")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("un fichier manquant doit provoquer une erreur")
	}
}
