// Package catalog es el cliente HTTP del servicio de presentaciones e implementa hub.Catalog.
// Es la única vía por la que el servicio realtime conoce las presentaciones: los microservicios
// no comparten disco ni base de datos, se comunican por su API.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"spd.web/services/realtime/internal/hub"
)

var _ hub.Catalog = (*Client)(nil)

// Client consulta los metadatos de las presentaciones.
type Client struct {
	base *url.URL
	http *http.Client
}

// New crea un cliente contra base (p. ej. http://localhost:8081) con un timeout por petición.
func New(base *url.URL, timeout time.Duration) *Client {
	return &Client{base: base, http: &http.Client{Timeout: timeout}}
}

// SlideCount devuelve el número de diapositivas consultando GET /api/presentaciones/{id}.
func (c *Client) SlideCount(ctx context.Context, presentationID string) (int, error) {
	u := c.base.JoinPath("api", "presentaciones", presentationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("catálogo de presentaciones: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10)) // permite reutilizar la conexión
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return 0, hub.ErrPresentationNotFound
	default:
		return 0, fmt.Errorf("catálogo de presentaciones: respuesta inesperada %s", resp.Status)
	}

	// Lector tolerante: solo interesa slide_count; el resto de campos se ignora.
	var body struct {
		SlideCount int `json:"slide_count"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return 0, fmt.Errorf("catálogo de presentaciones: respuesta inválida: %w", err)
	}
	if body.SlideCount < 1 {
		return 0, fmt.Errorf("catálogo de presentaciones: slide_count inválido (%d)", body.SlideCount)
	}
	return body.SlideCount, nil
}
