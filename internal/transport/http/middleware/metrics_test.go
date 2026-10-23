package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRecordsRegisteredRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Metrics())
	router.GET("/ping/:id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping/42", nil))

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "ifter_http_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			labels := make(map[string]string, len(metric.Label))
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["method"] == http.MethodGet && labels["route"] == "/ping/:id" && labels["status"] == "204" {
				return
			}
		}
	}
	t.Fatal("request metrics for /ping/:id with status 204 not found")
}
