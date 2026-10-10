package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"spd.web/services/rooms/internal/domain"
)

// createUser atiende POST /api/users.
func (h *Handler) createUser(c *gin.Context) {
	var in domain.NewUser
	if !bind(c, &in) {
		return
	}
	u, err := h.svc.CreateUser(c.Request.Context(), in)
	if err == nil {
		c.Header("Location", fmt.Sprintf("/api/users/%d", u.ID))
	}
	h.reply(c, http.StatusCreated, u, err)
}

// listUsers atiende GET /api/users.
func (h *Handler) listUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	h.reply(c, http.StatusOK, users, err)
}

// getUser atiende GET /api/users/:id.
func (h *Handler) getUser(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	u, err := h.svc.GetUser(c.Request.Context(), id)
	h.reply(c, http.StatusOK, u, err)
}

// updateUser atiende PATCH /api/users/:id.
func (h *Handler) updateUser(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in domain.UserChanges
	if !bind(c, &in) {
		return
	}
	u, err := h.svc.UpdateUser(c.Request.Context(), id, in)
	h.reply(c, http.StatusOK, u, err)
}

// deleteUser atiende DELETE /api/users/:id.
func (h *Handler) deleteUser(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	h.noContent(c, h.svc.DeleteUser(c.Request.Context(), id))
}

// userRooms atiende GET /api/users/:id/rooms: las salas de las que el usuario es miembro.
func (h *Handler) userRooms(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	rooms, err := h.svc.UserRooms(c.Request.Context(), id)
	h.reply(c, http.StatusOK, rooms, err)
}

// userGroups atiende GET /api/users/:id/groups: los grupos de los que el usuario es integrante.
func (h *Handler) userGroups(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	groups, err := h.svc.UserGroups(c.Request.Context(), id)
	h.reply(c, http.StatusOK, groups, err)
}
