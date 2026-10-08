// Package api expone el Hub por HTTP: el WebSocket de los visores y el endpoint de control
// del presentador.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
	"spd.web/services/realtime/internal/hub"
)

const maxControlBody = 1 << 10 // 1 KiB: un comando es un JSON diminuto

// Handler agrupa los endpoints del servicio realtime.
type Handler struct {
	hub            *hub.Hub
	originPatterns []string
	log            *slog.Logger

	// conns cuenta las conexiones WebSocket activas para poder esperarlas al apagar: el servidor
	// HTTP no las rastrea porque están "secuestradas" (hijacked).
	conns sync.WaitGroup

	// connCtx es el contexto de todas las conexiones WebSocket. forceClose lo cancela al final
	// del apagado para cerrar en el acto los sockets cuyo visor no respondió al cierre ordenado.
	connCtx    context.Context
	forceClose context.CancelFunc
}

// New crea el Handler. allowedOrigins son los orígenes (p. ej. "http://localhost:5173") desde
// los que se aceptan WebSockets, además del propio host (protección CSWSH).
func New(h *hub.Hub, allowedOrigins []string, log *slog.Logger) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Handler{hub: h, originPatterns: allowedOrigins, log: log, connCtx: ctx, forceClose: cancel}
}

// Register registra las rutas del servicio.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/ws/presentation/:id", h.serveWS)
	r.POST("/api/presentations/:id/control", h.control)
}

// Shutdown termina las conexiones WebSocket al apagar el servicio. El cierre ordenado (1001)
// lo inicia el Hub al detenerse; aquí se espera hasta grace a que cada visor lo confirme y,
// pasado ese plazo, se fuerza el cierre de los que no respondieron, para que un cliente
// defectuoso no retrase el apagado. Llamar cuando el servidor ya no acepta conexiones nuevas.
// Devuelve false si aun así quedaron conexiones abiertas.
func (h *Handler) Shutdown(grace time.Duration) bool {
	if h.waitConns(grace) {
		return true
	}
	h.forceClose()
	return h.waitConns(time.Second)
}

func (h *Handler) waitConns(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		h.conns.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// controlRequest es el cuerpo de POST /api/presentations/:id/control. Formatos aceptados:
//
//	{"action": "next"}   {"action": "prev"}
//	{"slide": 3}   {"slide_index": 3}   {"action": "goto", "slide": 3}
//
// "slide" y "slide_index" son sinónimos y empiezan en 1, igual que los archivos (001.png).
type controlRequest struct {
	Action     string `json:"action"`
	Slide      *int   `json:"slide"`
	SlideIndex *int   `json:"slide_index"`
}

func (r controlRequest) command() (hub.Command, error) {
	target := r.Slide
	if r.SlideIndex != nil {
		if target != nil {
			return hub.Command{}, errors.New(`usa "slide" o "slide_index", no ambos`)
		}
		target = r.SlideIndex
	}

	switch action := hub.Action(strings.ToLower(strings.TrimSpace(r.Action))); action {
	case hub.ActionNext, hub.ActionPrev:
		if target != nil {
			return hub.Command{}, fmt.Errorf("la acción %q no admite número de diapositiva", action)
		}
		return hub.Command{Action: action}, nil
	case "", hub.ActionGoto:
		if target == nil && action == hub.ActionGoto {
			return hub.Command{}, errors.New(`la acción "goto" requiere "slide"`)
		}
		if target == nil {
			return hub.Command{}, errors.New(`indica "action" (next, prev, goto) o "slide"`)
		}
		return hub.Command{Action: hub.ActionGoto, Slide: *target}, nil
	default:
		return hub.Command{}, fmt.Errorf("acción desconocida %q: usa next, prev o goto", r.Action)
	}
}

// control atiende POST /api/presentations/:id/control: actualiza el estado en memoria y el
// Hub difunde la nueva diapositiva a todos los visores de la sesión.
func (h *Handler) control(c *gin.Context) {
	id, ok := platform.CanonicalUUID(c.Param("id"))
	if !ok {
		platform.WriteError(c, http.StatusBadRequest, "id de presentación inválido")
		return
	}

	var req controlRequest
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxControlBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		platform.WriteError(c, http.StatusBadRequest, "JSON inválido: "+err.Error())
		return
	}
	cmd, err := req.command()
	if err != nil {
		platform.WriteError(c, http.StatusBadRequest, err.Error())
		return
	}

	state, err := h.hub.Control(c.Request.Context(), id, cmd)
	if err != nil {
		status, msg := controlError(err)
		if status >= http.StatusInternalServerError {
			h.log.Error("comando de control fallido", "presentation_id", id, "err", err)
		}
		platform.WriteError(c, status, msg)
		return
	}
	c.JSON(http.StatusOK, state)
}

func controlError(err error) (int, string) {
	switch {
	case errors.Is(err, hub.ErrPresentationNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, hub.ErrSlideOutOfRange), errors.Is(err, hub.ErrUnknownAction):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, hub.ErrClosed):
		return http.StatusServiceUnavailable, "servicio reiniciándose, reintenta"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusServiceUnavailable, "petición cancelada"
	default:
		// Normalmente, el servicio de presentaciones no respondió.
		return http.StatusServiceUnavailable, "no se pudo consultar el servicio de presentaciones"
	}
}
