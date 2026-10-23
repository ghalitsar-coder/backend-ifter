package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const RequestIDHeader = "X-Request-ID"

const requestIDKey = "request_id"

// Envelope is the single response shape for every endpoint. Keeping success and
// error envelopes identical makes clients and generated SDKs predictable.
type Envelope struct {
	Data  any        `json:"data,omitempty"`
	Meta  *Meta      `json:"meta,omitempty"`
	Error *ErrorBody `json:"error,omitempty"`
}

type Meta struct {
	RequestID string `json:"request_id,omitempty"`
	Page      int    `json:"page,omitempty"`
	PerPage   int    `json:"per_page,omitempty"`
	Total     *int64 `json:"total,omitempty"`
}

type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func SetRequestID(c *gin.Context, id string) {
	c.Set(requestIDKey, id)
}

func RequestID(c *gin.Context) string {
	value, ok := c.Get(requestIDKey)
	if !ok {
		return ""
	}
	id, _ := value.(string)
	return id
}

func JSON(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{
		Data: data,
		Meta: &Meta{RequestID: RequestID(c)},
	})
}

func JSONMeta(c *gin.Context, status int, data any, meta *Meta) {
	if meta == nil {
		meta = &Meta{}
	}
	meta.RequestID = RequestID(c)
	c.JSON(status, Envelope{Data: data, Meta: meta})
}

func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, Envelope{
		Error: &ErrorBody{
			Code:      code,
			Message:   message,
			RequestID: RequestID(c),
		},
	})
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}
