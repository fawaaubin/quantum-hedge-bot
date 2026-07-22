// Package logger fournit le logger structuré (slog) du bot.
// Il masque systématiquement les attributs sensibles connus.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Clés d'attributs interdites en clair dans les logs.
var sensitiveKeys = map[string]struct{}{
	"api_key":        {},
	"api_secret":     {},
	"signature":      {},
	"x-mbx-apikey":   {},
	"telegram_token": {},
}

// New construit un *slog.Logger selon le niveau ("debug", "info", "warn",
// "error") et le format ("json" ou "text") demandés. Tout attribut dont la
// clé est sensible est remplacé par "[REDACTED]".
func New(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
				a.Value = slog.StringValue("[REDACTED]")
			}
			return a
		},
	}

	var handler slog.Handler
	if strings.ToLower(format) == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
