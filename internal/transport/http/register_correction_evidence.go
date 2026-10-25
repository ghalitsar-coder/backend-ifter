package httptransport

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterCorrectionEvidence wires supporting evidence (documents/photos) that a
// correcting user attaches to an attendance correction.
// See docs/API_ADDENDUM.md §6.6.
//
// Owner: correction-evidence backend agent (migration 000014).
//
// Evidence belongs to the attendance-correction flow, so it rides the
// attendance-ops capability group (admin + teacher + duty_teacher; principal is
// excluded exactly like POST /attendance/records/:id/corrections).
//
//	groups.AttendanceOps (admin + teacher + duty_teacher)
//	  POST   /api/v1/attendance/records/:id/corrections/evidence   (multipart: file, note?)
//	  GET    /api/v1/attendance/records/:id/corrections/evidence
//	  GET    /api/v1/attendance/corrections/evidence/:evidenceId/download
//	  DELETE /api/v1/attendance/corrections/evidence/:evidenceId
//
// Storage reuses the imports directory and imports.max_file_size
// (docs/API_ADDENDUM.md §3): private disk, 0700 dir / 0600 file, never served
// as a static route. Content-type/extension and size are validated and
// evidence add/delete are audited inside the service.
//
// Wiring note: this package does not own router.go; the service arrives
// through FeatureDeps. router.New() builds it on the shared API pool with the
// shared imports storage settings (API_ADDENDUM.md §3) — no feature-owned
// pools. Without a wired service (fake readiness, register tests) the surface
// stays disabled.

func RegisterCorrectionEvidence(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	service := deps.CorrectionEvidence
	if service == nil {
		return
	}
	handler := newCorrectionEvidenceHandler(service, log)
	uploadLimit := service.MaxFileSize() + (1 << 20)

	groups.AttendanceOps.POST("/attendance/records/:id/corrections/evidence",
		middleware.BodyLimit(uploadLimit), handler.Upload)
	groups.AttendanceOps.GET("/attendance/records/:id/corrections/evidence", handler.List)
	groups.AttendanceOps.GET("/attendance/corrections/evidence/:evidenceId/download", handler.Download)
	groups.AttendanceOps.DELETE("/attendance/corrections/evidence/:evidenceId", handler.Delete)
}
