// Servicio realtime: dominio de las sesiones en vivo. Mantiene en memoria la diapositiva
// actual de cada presentación (Hub), atiende el WebSocket de los visores y el endpoint de
// control del presentador. Conoce las presentaciones consultando al servicio presentations.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"spd.web/internal/platform"
	"spd.web/services/realtime/internal/api"
	"spd.web/services/realtime/internal/catalog"
	"spd.web/services/realtime/internal/hub"
)

func main() {
	var env platform.Env
	log := platform.NewLogger("realtime", &env)
	if err := run(log, &env); err != nil {
		log.Error("el servicio terminó con error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, env *platform.Env) error {
	addr := env.String("HTTP_ADDR", ":8082")
	presentationsURL := env.URL("PRESENTATIONS_URL", "http://localhost:8081")
	allowedOrigins := env.List("ALLOWED_ORIGINS", []string{"http://localhost:5173"})
	idleTTL := env.Duration("SESSION_IDLE_TTL", 24*time.Hour)
	if err := env.Err(); err != nil {
		return fmt.Errorf("configuración inválida: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	h := hub.New(catalog.New(presentationsURL, 5*time.Second), hub.Options{IdleTTL: idleTTL, Logger: log})
	hubCtx, stopHub := context.WithCancel(context.Background())
	defer stopHub()
	go h.Run(hubCtx)

	handler := api.New(h, allowedOrigins, log)
	engine := platform.NewEngine(log)
	handler.Register(engine)

	srv := platform.NewServer(addr, engine, log)
	// http.Server.Shutdown no cierra ni espera las conexiones WebSocket (están secuestradas).
	// Al empezar el apagado se detiene el Hub: cierra todos los buzones y cada conexión envía
	// a su visor un cierre ordenado 1001 ("going away").
	srv.RegisterOnShutdown(stopHub)

	err := platform.Serve(ctx, srv, log)
	stopHub()
	<-h.Done()
	if !handler.Shutdown(3 * time.Second) {
		log.Warn("algunas conexiones WebSocket no terminaron a tiempo")
	}
	return err
}
