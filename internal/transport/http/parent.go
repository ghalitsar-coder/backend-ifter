package httptransport

import (
	"net/http"

	"github.com/gin-gonic/gin"

	studentaccountspkg "github.com/ghalitsar-coder/backend-ifter/internal/studentaccounts"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// parentLinkRequest is one child in the PUT /users/:id/children payload.
type parentLinkRequest struct {
	StudentID    string  `json:"student_id"`
	Relationship *string `json:"relationship"`
}

type parentLinksRequest struct {
	Children []parentLinkRequest `json:"children" binding:"required"`
}

// ParentChildren handles GET /parent/children -> 200 { "data": [...] }.
func (h *studentAccountHandler) ParentChildren(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.ParentChildren(c.Request.Context(), actor)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// ParentAttendance handles GET /parent/children/:id/attendance.
func (h *studentAccountHandler) ParentAttendance(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.ParentAttendance(c.Request.Context(), actor, c.Param("id"), filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// ParentAttendanceSummary handles GET /parent/children/:id/attendance-summary.
func (h *studentAccountHandler) ParentAttendanceSummary(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.ParentAttendanceSummary(c.Request.Context(), actor, c.Param("id"), filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// ParentDiscipline handles GET /parent/children/:id/discipline.
func (h *studentAccountHandler) ParentDiscipline(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.ParentDiscipline(c.Request.Context(), actor, c.Param("id"), filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// ParentDisciplineSummary handles GET /parent/children/:id/discipline-summary.
func (h *studentAccountHandler) ParentDisciplineSummary(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.ParentDisciplineSummary(c.Request.Context(), actor, c.Param("id"), filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// ParentDispensations handles GET /parent/children/:id/dispensations ->
// 200 { "data": [...], "total", "limit", "offset" }.
func (h *studentAccountHandler) ParentDispensations(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, offset, err := parseStudentPortalPage(c)
	if err != nil {
		h.writeError(c, studentaccountspkg.ErrInvalidInput)
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.ParentDispensations(c.Request.Context(), actor, c.Param("id"), limit, offset)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// AdminListChildren handles GET /users/:id/children (admin only).
func (h *studentAccountHandler) AdminListChildren(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.AdminListParentLinks(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// AdminSetChildren handles PUT /users/:id/children (admin only): the payload
// replaces the whole link set and is audited as user.children.update.
func (h *studentAccountHandler) AdminSetChildren(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request parentLinksRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, studentaccountspkg.ErrInvalidInput)
		return
	}
	links := make([]studentaccountspkg.ParentLinkInput, 0, len(request.Children))
	for _, child := range request.Children {
		relationship := ""
		if child.Relationship != nil {
			relationship = *child.Relationship
		}
		links = append(links, studentaccountspkg.ParentLinkInput{
			StudentID:    child.StudentID,
			Relationship: relationship,
		})
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.AdminSetParentLinks(c.Request.Context(), actor, c.Param("id"), links, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}
