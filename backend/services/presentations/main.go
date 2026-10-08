// Servicio presentations: dominio de los archivos. Recibe las subidas (.pptx o .pdf), renderiza
// cada diapositiva como PNG en <UPLOAD_DIR>/<uuid>/ y sirve las imágenes.
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
	"spd.web/services/presentations/internal/api"
	"spd.web/services/presentations/internal/convert"
	"spd.web/services/presentations/internal/presentation"
	"spd.web/services/presentations/internal/storage"
)

const mb = 1 << 20

func main() {
	var env platform.Env
	dotenv := env.LoadDotEnv(".env")
	log := platform.NewLogger("presentations", &env)
	if dotenv {
		log.Info("variables de entorno cargadas", "archivo", ".env")
	}
	if err := run(log, &env); err != nil {
		log.Error("el servicio terminó con error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, env *platform.Env) error {
	addr := env.String("HTTP_ADDR", ":8081")
	uploadDir := env.String("UPLOAD_DIR", "./uploads")
	limits := storage.Limits{
		MaxUploadBytes: int64(env.Int("MAX_UPLOAD_MB", 100)) * mb,
		MaxSlides:      env.Int("MAX_SLIDES", 500),
		MaxTotalBytes:  int64(env.Int("MAX_EXTRACTED_MB", 1024)) * mb,
		MaxEntries:     10_000,
	}
	convOpts := convert.Options{
		SofficeBin:    env.String("SOFFICE_BIN", "soffice"),
		PdftoppmBin:   env.String("PDFTOPPM_BIN", "pdftoppm"),
		Timeout:       env.Duration("CONVERT_TIMEOUT", 3*time.Minute),
		MaxConcurrent: env.Int("MAX_CONCURRENT_CONVERSIONS", 2),
		MaxPixels:     env.Int("RENDER_MAX_PX", 1920),
	}
	if err := env.Err(); err != nil {
		return fmt.Errorf("configuración inválida: %w", err)
	}

	store, err := storage.Open(uploadDir, log)
	if err != nil {
		return err
	}
	log.Info("almacenamiento listo", "dir", store.Root(), "max_upload", storage.FormatBytes(limits.MaxUploadBytes))

	conv, err := convert.NewLibreOffice(convOpts)
	if err != nil {
		return err
	}
	defer conv.Close()
	if err := conv.Check(); err != nil {
		log.Warn("conversión incompleta: las subidas afectadas responderán 503", "err", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc := presentation.NewService(store, conv, limits, log)
	engine := platform.NewEngine(log)
	api.New(svc, limits.MaxUploadBytes, log).Register(engine)

	return platform.Serve(ctx, platform.NewServer(addr, engine, log), log)
}
