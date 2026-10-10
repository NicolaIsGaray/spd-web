// Servicio gateway: punto de entrada único de la API. Enruta cada endpoint público al
// microservicio de su dominio (presentations, realtime o rooms) y gestiona CORS.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"spd.web/internal/platform"
	"spd.web/services/gateway/internal/gateway"
)

func main() {
	var env platform.Env
	dotenv := env.LoadDotEnv(".env")
	log := platform.NewLogger("gateway", &env)
	if dotenv {
		log.Info("variables de entorno cargadas", "archivo", ".env")
	}
	if err := run(log, &env); err != nil {
		log.Error("el servicio terminó con error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, env *platform.Env) error {
	addr := env.String("HTTP_ADDR", ":8080")
	upstreams := gateway.Upstreams{
		Presentations: env.URL("PRESENTATIONS_URL", "http://localhost:8081"),
		Realtime:      env.URL("REALTIME_URL", "http://localhost:8082"),
		Rooms:         env.URL("ROOMS_URL", "http://localhost:8083"),
	}
	allowedOrigins := env.List("ALLOWED_ORIGINS", []string{"http://localhost:5173"})
	if err := env.Err(); err != nil {
		return fmt.Errorf("configuración inválida: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	engine := platform.NewEngine(log, gateway.CORS(allowedOrigins))
	gateway.Register(engine, upstreams, log)
	log.Info("rutas publicadas", "presentations", upstreams.Presentations.String(), "realtime", upstreams.Realtime.String(),
		"rooms", upstreams.Rooms.String())

	return platform.Serve(ctx, platform.NewServer(addr, engine, log), log)
}
