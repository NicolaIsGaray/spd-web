package platform

import (
	"log/slog"
	"os"
)

// NewLogger crea el logger estructurado (log/slog) del servicio. Variables de entorno:
//
//	LOG_LEVEL   debug | info (defecto) | warn | error
//	LOG_FORMAT  text (defecto) | json
func NewLogger(service string, env *Env) *slog.Logger {
	level := slog.LevelInfo
	if raw := env.String("LOG_LEVEL", "info"); level.UnmarshalText([]byte(raw)) != nil {
		env.fail("LOG_LEVEL", raw, "debug, info, warn o error")
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch format := env.String("LOG_FORMAT", "text"); format {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		if format != "text" {
			env.fail("LOG_FORMAT", format, "text o json")
		}
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(handler).With("service", service)
}
