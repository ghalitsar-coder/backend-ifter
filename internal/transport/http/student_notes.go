package httptransport

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// studentNoteRequest is the POST /students/:id/notes payload (FR-C03).
// Note length (<= 2000 runes) and non-blank content are validated by the
// masterdata service so the same rule holds for every caller.
type studentNoteRequest struct {
	Note string `json:"note" binding:"required"`
}

// ListStudentNotes handles GET /students/:id/notes -> 200 { "data": [...] },
// newest first.
func (h *masterDataHandler) ListStudentNotes(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	notes, err := h.service.ListStudentNotes(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": notes})
}

// CreateStudentNote handles POST /students/:id/notes -> 201 { "note": {...} },
// audited as student_note.create.
func (h *masterDataHandler) CreateStudentNote(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req studentNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "note is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	note, err := h.service.CreateStudentNote(c.Request.Context(), actor, c.Param("id"), req.Note, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"note": note})
}

// DeleteStudentNote handles DELETE /students/:id/notes/:noteId -> 204,
// audited as student_note.delete.
func (h *masterDataHandler) DeleteStudentNote(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.DeleteStudentNote(c.Request.Context(), actor, c.Param("id"), c.Param("noteId"), response.RequestID(c)); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
