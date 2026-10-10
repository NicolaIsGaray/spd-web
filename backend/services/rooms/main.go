// Servicio rooms: dominio de las salas. Guarda en PostgreSQL (con GORM) los usuarios, las salas
// y sus miembros, los grupos de cada sala y las presentaciones asignadas a cada grupo, que
// valida contra el servicio presentations.
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
	"spd.web/services/rooms/internal/api"
	"spd.web/services/rooms/internal/catalog"
	"spd.web/services/rooms/internal/domain"
	"spd.web/services/rooms/internal/store"
)

func main() {
	var env platform.Env
	// El .env se carga antes de leer cualquier variable, incluidas las del logger.
	dotenv := env.LoadDotEnv(".env")
	log := platform.NewLogger("rooms", &env)
	if dotenv {
		log.Info("variables de entorno cargadas", "archivo", ".env")
	}
	if err := run(log, &env); err != nil {
		log.Error("el servicio terminó con error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, env *platform.Env) error {
	addr := env.String("HTTP_ADDR", ":8083")
	dsn := env.String("DATABASE_URL", "postgres://spd:spd@localhost:5432/spd?sslmode=disable")
	maxConns := env.Int("DB_MAX_CONNS", 10)
	presentationsURL := env.URL("PRESENTATIONS_URL", "http://localhost:8081")
	if err := env.Err(); err != nil {
		return fmt.Errorf("configuración inválida: %w", err)
	}

	db, err := store.Open(dsn, maxConns, log)
	if err != nil {
		return err
	}
	defer store.Close(db)
	if err := store.Migrate(db); err != nil {
		return err
	}
	log.Info("base de datos lista", "database", db.Migrator().CurrentDatabase())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc := domain.NewService(store.New(db), catalog.New(presentationsURL, 5*time.Second))
	engine := platform.NewEngine(log)
	api.New(svc, log).Register(engine)

	return platform.Serve(ctx, platform.NewServer(addr, engine, log), log)
}
