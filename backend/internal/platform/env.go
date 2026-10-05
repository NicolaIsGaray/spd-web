// Package platform reúne la infraestructura común a todos los microservicios: configuración
// por variables de entorno, logging estructurado y servidor HTTP (Gin) con apagado ordenado.
// No contiene lógica de dominio: cada servicio es dueño de la suya.
package platform

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env lee la configuración de variables de entorno con valores por defecto. En lugar de
// abortar en el primer valor inválido, acumula los errores para informar de todos a la vez
// al arrancar (fail fast).
type Env struct {
	errs []error
}

// String devuelve la variable key, o def si no está definida o está vacía.
func (e *Env) String(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// Int devuelve la variable key como entero positivo.
func (e *Env) Int(key string, def int) int {
	raw := e.String(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		e.fail(key, raw, "un entero positivo")
		return def
	}
	return n
}

// Duration devuelve la variable key como duración de Go ("30s", "5m", "24h").
func (e *Env) Duration(key string, def time.Duration) time.Duration {
	raw := e.String(key, "")
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		e.fail(key, raw, "una duración como 30s, 5m o 24h")
		return def
	}
	return d
}

// List devuelve la variable key como lista separada por comas. Definida pero vacía equivale
// a una lista vacía (sirve para anular un valor por defecto).
func (e *Env) List(key string, def []string) []string {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	var out []string
	for item := range strings.SplitSeq(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// URL devuelve la variable key como URL http(s) absoluta.
func (e *Env) URL(key, def string) *url.URL {
	raw := e.String(key, def)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		e.fail(key, raw, "una URL absoluta http(s)")
		return &url.URL{}
	}
	return u
}

// Err devuelve todos los errores de configuración encontrados, o nil.
func (e *Env) Err() error {
	return errors.Join(e.errs...)
}

func (e *Env) fail(key, raw, want string) {
	e.errs = append(e.errs, fmt.Errorf("%s=%q: se esperaba %s", key, raw, want))
}
