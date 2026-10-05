// Package gateway implementa el API Gateway: el único punto de entrada para el frontend.
// Enruta cada endpoint público al microservicio dueño de su dominio, de modo que los
// servicios internos pueden moverse, escalar o desplegarse por separado sin que el cliente
// lo note.
package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

// Upstreams son las URLs internas de los microservicios de dominio.
type Upstreams struct {
	Presentations *url.URL
	Realtime      *url.URL
}

// Register publica la tabla de rutas de la API.
func Register(r gin.IRouter, up Upstreams, log *slog.Logger) {
	transport := newTransport()
	presentations := gin.WrapH(newProxy(up.Presentations, "presentations", transport, log))
	realtime := gin.WrapH(newProxy(up.Realtime, "realtime", transport, log))

	// Dominio "presentaciones": subida, metadatos e imágenes.
	r.POST("/api/presentaciones/upload", presentations)
	r.GET("/api/presentaciones/:id", presentations)
	r.GET("/api/presentaciones/:id/slides/:slide_id", presentations)

	// Dominio "tiempo real": control del presentador y WebSocket de los visores.
	r.POST("/api/presentaciones/:id/control", realtime)
	r.GET("/ws/presentacion/:id", realtime)
}

// newProxy crea un proxy inverso hacia target. httputil.ReverseProxy soporta de forma nativa
// el upgrade a WebSocket: tras el 101 copia bytes en ambos sentidos hasta que alguien cierra.
func newProxy(target *url.URL, name string, transport http.RoundTripper, log *slog.Logger) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded() // X-Forwarded-For/-Host/-Proto (los entrantes se descartan)
			// Conservar el Host público: el servicio realtime lo compara con la cabecera Origin
			// al aceptar el WebSocket (protección CSWSH). Sin esto vería el host interno.
			r.Out.Host = r.In.Host
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return // el cliente se fue: nadie leerá la respuesta
			}
			log.Error("microservicio no disponible", "upstream", name, "path", r.URL.Path, "err", err)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"servicio no disponible"}`))
		},
	}
}

func newTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	// El valor por defecto (2) haría que el gateway abriera y cerrara conexiones sin parar
	// hacia cada servicio con tráfico concurrente.
	t.MaxIdleConnsPerHost = 64
	return t
}
