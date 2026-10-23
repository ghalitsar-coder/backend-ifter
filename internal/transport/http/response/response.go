package response

import "github.com/gin-gonic/gin"

const (
	RequestIDHeader = "X-Request-ID"
	requestIDKey    = "request_id"
)

type ErrorResponse struct {
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

func JSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: RequestID(c),
	})
}
