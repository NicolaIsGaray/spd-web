package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"spd.web/services/rooms/internal/domain"
)

// createRoom atiende POST /api/rooms.
func (h *Handler) createRoom(c *gin.Context) {
	var in domain.NewRoom
	if !bind(c, &in) {
		return
	}
	r, err := h.svc.CreateRoom(c.Request.Context(), in)
	if err == nil {
		c.Header("Location", fmt.Sprintf("/api/rooms/%d", r.ID))
	}
	h.reply(c, http.StatusCreated, r, err)
}

// listRooms atiende GET /api/rooms.
func (h *Handler) listRooms(c *gin.Context) {
	rooms, err := h.svc.ListRooms(c.Request.Context())
	h.reply(c, http.StatusOK, rooms, err)
}

// getRoom atiende GET /api/rooms/:id.
func (h *Handler) getRoom(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	r, err := h.svc.GetRoom(c.Request.Context(), id)
	h.reply(c, http.StatusOK, r, err)
}

// updateRoom atiende PATCH /api/rooms/:id.
func (h *Handler) updateRoom(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in domain.RoomChanges
	if !bind(c, &in) {
		return
	}
	r, err := h.svc.UpdateRoom(c.Request.Context(), id, in)
	h.reply(c, http.StatusOK, r, err)
}

// deleteRoom atiende DELETE /api/rooms/:id.
func (h *Handler) deleteRoom(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	h.noContent(c, h.svc.DeleteRoom(c.Request.Context(), id))
}

// roomMembers atiende GET /api/rooms/:id/members.
func (h *Handler) roomMembers(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	users, err := h.svc.RoomMembers(c.Request.Context(), id)
	h.reply(c, http.StatusOK, users, err)
}

// joinRoom atiende POST /api/rooms/:id/members: 201 si el usuario entra (o vuelve) a la sala,
// 200 si ya era miembro.
func (h *Handler) joinRoom(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in domain.Join
	if !bind(c, &in) {
		return
	}
	m, created, err := h.svc.JoinRoom(c.Request.Context(), id, in)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	h.reply(c, status, m, err)
}

// leaveRoom atiende DELETE /api/rooms/:id/members/:user_id.
func (h *Handler) leaveRoom(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	userID, ok := idParam(c, "user_id")
	if !ok {
		return
	}
	h.noContent(c, h.svc.LeaveRoom(c.Request.Context(), id, userID))
}

// roomGroups atiende GET /api/rooms/:id/groups.
func (h *Handler) roomGroups(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	groups, err := h.svc.RoomGroups(c.Request.Context(), id)
	h.reply(c, http.StatusOK, groups, err)
}

// createGroup atiende POST /api/rooms/:id/groups.
func (h *Handler) createGroup(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in domain.GroupFields
	if !bind(c, &in) {
		return
	}
	g, err := h.svc.CreateGroup(c.Request.Context(), id, in)
	if err == nil {
		c.Header("Location", fmt.Sprintf("/api/groups/%d", g.ID))
	}
	h.reply(c, http.StatusCreated, g, err)
}
