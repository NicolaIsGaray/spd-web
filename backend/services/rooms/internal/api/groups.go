package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"spd.web/services/rooms/internal/domain"
)

// getGroup atiende GET /api/groups/:id.
func (h *Handler) getGroup(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	g, err := h.svc.GetGroup(c.Request.Context(), id)
	h.reply(c, http.StatusOK, g, err)
}

// updateGroup atiende PATCH /api/groups/:id.
func (h *Handler) updateGroup(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in domain.GroupFields
	if !bind(c, &in) {
		return
	}
	g, err := h.svc.UpdateGroup(c.Request.Context(), id, in)
	h.reply(c, http.StatusOK, g, err)
}

// deleteGroup atiende DELETE /api/groups/:id.
func (h *Handler) deleteGroup(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	h.noContent(c, h.svc.DeleteGroup(c.Request.Context(), id))
}

// groupMembers atiende GET /api/groups/:id/members.
func (h *Handler) groupMembers(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	users, err := h.svc.GroupMembers(c.Request.Context(), id)
	h.reply(c, http.StatusOK, users, err)
}

// joinGroup atiende POST /api/groups/:id/members: 201 si el usuario entra (o vuelve) al grupo,
// 200 si ya era integrante.
func (h *Handler) joinGroup(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in domain.GroupJoin
	if !bind(c, &in) {
		return
	}
	m, created, err := h.svc.JoinGroup(c.Request.Context(), id, in)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	h.reply(c, status, m, err)
}

// leaveGroup atiende DELETE /api/groups/:id/members/:user_id.
func (h *Handler) leaveGroup(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	userID, ok := idParam(c, "user_id")
	if !ok {
		return
	}
	h.noContent(c, h.svc.LeaveGroup(c.Request.Context(), id, userID))
}

// groupPresentations atiende GET /api/groups/:id/presentations.
func (h *Handler) groupPresentations(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	presentations, err := h.svc.GroupPresentations(c.Request.Context(), id)
	h.reply(c, http.StatusOK, presentations, err)
}

// addPresentation atiende POST /api/groups/:id/presentations.
func (h *Handler) addPresentation(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in domain.NewPresentation
	if !bind(c, &in) {
		return
	}
	p, err := h.svc.AddPresentation(c.Request.Context(), id, in)
	if err == nil {
		c.Header("Location", fmt.Sprintf("/api/groups/%d/presentations/%s", id, p.ID))
	}
	h.reply(c, http.StatusCreated, p, err)
}

// getPresentation atiende GET /api/groups/:id/presentations/:presentation_id.
func (h *Handler) getPresentation(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	pid, ok := presentationParam(c)
	if !ok {
		return
	}
	p, err := h.svc.GetPresentation(c.Request.Context(), id, pid)
	h.reply(c, http.StatusOK, p, err)
}

// updatePresentation atiende PATCH /api/groups/:id/presentations/:presentation_id.
func (h *Handler) updatePresentation(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	pid, ok := presentationParam(c)
	if !ok {
		return
	}
	var in domain.PresentationChanges
	if !bind(c, &in) {
		return
	}
	p, err := h.svc.UpdatePresentation(c.Request.Context(), id, pid, in)
	h.reply(c, http.StatusOK, p, err)
}

// removePresentation atiende DELETE /api/groups/:id/presentations/:presentation_id.
func (h *Handler) removePresentation(c *gin.Context) {
	id, ok := idParam(c, "id")
	if !ok {
		return
	}
	pid, ok := presentationParam(c)
	if !ok {
		return
	}
	h.noContent(c, h.svc.RemovePresentation(c.Request.Context(), id, pid))
}
