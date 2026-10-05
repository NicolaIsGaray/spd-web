package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute

	// requestTimeout acota la lectura y la escritura de cada petición HTTP normal (no WebSocket).
	// Es generoso porque cubre subir un archivo grande y convertir un PPTX.
	requestTimeout = 10 * time.Minute

	shutdownGrace = 20 * time.Second // espera a las peticiones en curso al apagar
	shutdownForce = 5 * time.Second  // margen extra tras cancelarlas
)

// NewEngine crea un *gin.Engine con el middleware común (recuperación de pánicos, log de
// peticiones y plazos por petición), el middleware extra indicado, GET /healthz y un 404 JSON.
func NewEngine(log *slog.Logger, middleware ...gin.HandlerFunc) *gin.Engine {
	engine := gin.New()
	// Ningún proxy de confianza: ClientIP() usa la IP real de la conexión y no cabeceras
	// X-Forwarded-For que cualquiera podría falsificar.
	_ = engine.SetTrustedProxies(nil)
	engine.Use(gin.Recovery(), requestLogger(log), requestDeadlines(requestTimeout))
	engine.Use(middleware...)
	engine.NoRoute(func(c *gin.Context) { WriteError(c, http.StatusNotFound, "ruta no encontrada") })
	engine.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	return engine
}

// NewServer crea el http.Server. No fija ReadTimeout/WriteTimeout globales porque cortarían
// las conexiones WebSocket (viven horas); los plazos por petición los pone requestDeadlines.
func NewServer(addr string, handler http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// Serve atiende peticiones hasta que ctx se cancela y después apaga el servidor en dos fases:
//
//  1. Shutdown: deja de aceptar conexiones y espera hasta shutdownGrace a que terminen las
//     peticiones en curso. Las conexiones WebSocket no cuentan (están "secuestradas"): de
//     ellas se ocupa cada servicio con srv.RegisterOnShutdown.
//  2. Si el plazo se agota, cancela el contexto de las peticiones que siguen vivas (p. ej. una
//     conversión de PPTX) para que liberen recursos (procesos hijo, directorios temporales)
//     y les concede shutdownForce más.
func Serve(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	baseCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	srv.BaseContext = func(net.Listener) context.Context { return baseCtx }

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	log.Info("servidor HTTP escuchando", "addr", ln.Addr().String())

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return err // falló antes de que se pidiera el apagado
	case <-ctx.Done():
	}

	log.Info("apagando: esperando a las peticiones en curso", "plazo", shutdownGrace)
	graceCtx, cancelGrace := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancelGrace()
	err = srv.Shutdown(graceCtx)
	if errors.Is(err, context.DeadlineExceeded) {
		log.Warn("plazo agotado: cancelando las peticiones que siguen activas")
		cancelRequests()
		forceCtx, cancelForce := context.WithTimeout(context.Background(), shutdownForce)
		defer cancelForce()
		err = srv.Shutdown(forceCtx)
	}
	if err != nil {
		return fmt.Errorf("apagado incompleto: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("servidor HTTP detenido")
	return nil
}

// WriteError responde con un error JSON uniforme: {"error": "..."}.
func WriteError(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// IsWebSocketUpgrade indica si la petición es un handshake WebSocket.
func IsWebSocketUpgrade(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		headerHasToken(r.Header, "Connection", "upgrade") &&
		headerHasToken(r.Header, "Upgrade", "websocket")
}

func headerHasToken(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for t := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

// requestDeadlines fija plazos de lectura y escritura por petición (protección frente a
// clientes lentos tipo slowloris). Los handshakes WebSocket quedan sin plazo: esas conexiones
// viven horas y su salud la vigilan el ping/pong y el Hub del servicio realtime.
func requestDeadlines(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsWebSocketUpgrade(c.Request) {
			rc := http.NewResponseController(c.Writer)
			deadline := time.Now().Add(d)
			// En tests con httptest.ResponseRecorder no está soportado: se ignora el error.
			_ = rc.SetReadDeadline(deadline)
			_ = rc.SetWriteDeadline(deadline)
		}
		c.Next()
	}
}

func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		if status == http.StatusOK && c.Writer.Size() <= 0 && IsWebSocketUpgrade(c.Request) {
			// httputil.ReverseProxy (gateway) escribe el 101 directamente en la conexión
			// secuestrada, sin pasar por Gin, que seguiría informando de un 200.
			status = http.StatusSwitchingProtocols
		}
		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		case c.Request.URL.Path == "/healthz":
			level = slog.LevelDebug
		}
		log.LogAttrs(context.Background(), level, "petición HTTP",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
			slog.String("client_ip", c.ClientIP()),
		)
	}
}
