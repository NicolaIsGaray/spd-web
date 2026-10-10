// Package api expone el dominio "salas" por HTTP: usuarios, salas y sus miembros, grupos y sus
// integrantes, y las presentaciones asignadas a cada grupo.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"spd.web/internal/platform"
	"spd.web/services/rooms/internal/domain"
)

// maxBodyBytes acota el cuerpo JSON de las peticiones.
const maxBodyBytes = 1 << 20

// Handler agrupa los endpoints REST del servicio.
type Handler struct {
	svc *domain.Service
	log *slog.Logger
}

// New crea el Handler.
func New(svc *domain.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

// Register registra las rutas del servicio.
func (h *Handler) Register(r gin.IRouter) {
	r.POST("/api/users", h.createUser)
	r.GET("/api/users", h.listUsers)
	r.GET("/api/users/:id", h.getUser)
	r.PATCH("/api/users/:id", h.updateUser)
	r.DELETE("/api/users/:id", h.deleteUser)
	r.GET("/api/users/:id/rooms", h.userRooms)
	r.GET("/api/users/:id/groups", h.userGroups)

	r.POST("/api/rooms", h.createRoom)
	r.GET("/api/rooms", h.listRooms)
	r.GET("/api/rooms/:id", h.getRoom)
	r.PATCH("/api/rooms/:id", h.updateRoom)
	r.DELETE("/api/rooms/:id", h.deleteRoom)
	r.GET("/api/rooms/:id/members", h.roomMembers)
	r.POST("/api/rooms/:id/members", h.joinRoom)
	r.DELETE("/api/rooms/:id/members/:user_id", h.leaveRoom)
	r.GET("/api/rooms/:id/groups", h.roomGroups)
	r.POST("/api/rooms/:id/groups", h.createGroup)

	r.GET("/api/groups/:id", h.getGroup)
	r.PATCH("/api/groups/:id", h.updateGroup)
	r.DELETE("/api/groups/:id", h.deleteGroup)
	r.GET("/api/groups/:id/members", h.groupMembers)
	r.POST("/api/groups/:id/members", h.joinGroup)
	r.DELETE("/api/groups/:id/members/:user_id", h.leaveGroup)
	r.GET("/api/groups/:id/presentations", h.groupPresentations)
	r.POST("/api/groups/:id/presentations", h.addPresentation)
	r.GET("/api/groups/:id/presentations/:presentation_id", h.getPresentation)
	r.PATCH("/api/groups/:id/presentations/:presentation_id", h.updatePresentation)
	r.DELETE("/api/groups/:id/presentations/:presentation_id", h.removePresentation)
}

// idParam lee de la ruta un id numérico (1 o mayor) y, si no lo es, responde 400.
func idParam(c *gin.Context, name string) (uint, bool) {
	n, err := strconv.ParseUint(c.Param(name), 10, 63) // 63 bits: el máximo de un bigint
	if err != nil || n == 0 {
		platform.WriteError(c, http.StatusBadRequest, name+" inválido")
		return 0, false
	}
	return uint(n), true
}

// presentationParam lee de la ruta el UUID de una presentación y, si no lo es, responde 400.
func presentationParam(c *gin.Context) (string, bool) {
	id, ok := platform.CanonicalUUID(c.Param("presentation_id"))
	if !ok {
		platform.WriteError(c, http.StatusBadRequest, "presentation_id inválido")
	}
	return id, ok
}

// bind decodifica el cuerpo JSON en dst y, si no puede, responde 400 (o 413). Rechaza los
// campos desconocidos: un "fullname" mal escrito no debe ignorarse en silencio.
func bind(c *gin.Context, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil && dec.More() {
		err = errors.New("hay datos después del objeto JSON")
	}
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		platform.WriteError(c, http.StatusRequestEntityTooLarge, "el cuerpo supera 1 MB")
	} else {
		platform.WriteError(c, http.StatusBadRequest, "cuerpo JSON inválido: "+jsonError(err))
	}
	return false
}

// jsonError describe en español los errores habituales de encoding/json.
func jsonError(err error) string {
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		return "falta el cuerpo"
	case errors.As(err, &typeErr):
		return fmt.Sprintf("el campo %s debe ser de tipo %s", typeErr.Field, typeErr.Type)
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return "JSON mal formado"
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return "campo desconocido " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	}
	return err.Error()
}

// reply responde con v en JSON, o traduce err si lo hay.
func (h *Handler) reply(c *gin.Context, status int, v any, err error) {
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(status, v)
}

// noContent responde 204, o traduce err si lo hay.
func (h *Handler) noContent(c *gin.Context, err error) {
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// fail traduce los errores del dominio a respuestas HTTP. A los 5xx no se les adjunta el
// detalle (puede contener datos internos): se registra en el log.
func (h *Handler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		platform.WriteError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrWrongAccessKey):
		platform.WriteError(c, http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrUserNotFound), errors.Is(err, domain.ErrRoomNotFound),
		errors.Is(err, domain.ErrGroupNotFound), errors.Is(err, domain.ErrPresentationNotFound),
		errors.Is(err, domain.ErrNotRoomMember), errors.Is(err, domain.ErrNotGroupMember):
		platform.WriteError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrEmailTaken), errors.Is(err, domain.ErrPresentationAssigned),
		errors.Is(err, domain.ErrRoomMembershipRequired), errors.Is(err, domain.ErrGroupFull),
		errors.Is(err, domain.ErrLimitBelowMembers):
		platform.WriteError(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrPresentationUnknown):
		platform.WriteError(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrCatalogUnavailable):
		h.log.Warn("servicio presentations no disponible", "err", err)
		platform.WriteError(c, http.StatusServiceUnavailable, domain.ErrCatalogUnavailable.Error())
	case errors.Is(err, context.Canceled):
		platform.WriteError(c, http.StatusServiceUnavailable, "petición cancelada")
	default:
		h.log.Error("error interno", "path", c.Request.URL.Path, "err", err)
		platform.WriteError(c, http.StatusInternalServerError, "error interno")
	}
}
