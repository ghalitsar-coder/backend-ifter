package metrics

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ifter",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total HTTP requests by method, route, and status.",
	}, []string{"method", "route", "status"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "ifter",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP request latency by method and route.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	poolConnections = prometheus.NewDesc(
		"ifter_db_pool_connections",
		"Database pool connections by state.",
		[]string{"state"},
		nil,
	)
	poolMaxConnections = prometheus.NewDesc(
		"ifter_db_pool_max_connections",
		"Maximum database pool connections.",
		nil,
		nil,
	)
)

// ObserveRequest records one completed request. route must be the registered
// path pattern, never the raw URL, to keep metric cardinality bounded.
func ObserveRequest(method, route, status string, seconds float64) {
	requestsTotal.WithLabelValues(method, route, status).Inc()
	requestDuration.WithLabelValues(method, route).Observe(seconds)
}

func Handler() http.Handler {
	return promhttp.Handler()
}

func RegisterDBPool(pool *pgxpool.Pool) {
	prometheus.MustRegister(poolCollector{pool: pool})
}

type poolCollector struct {
	pool *pgxpool.Pool
}

func (c poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- poolConnections
	ch <- poolMaxConnections
}

func (c poolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(poolMaxConnections, prometheus.GaugeValue, float64(stat.MaxConns()))
	for state, value := range map[string]int32{
		"acquired":     stat.AcquiredConns(),
		"idle":         stat.IdleConns(),
		"total":        stat.TotalConns(),
		"constructing": stat.ConstructingConns(),
	} {
		ch <- prometheus.MustNewConstMetric(poolConnections, prometheus.GaugeValue, float64(value), state)
	}
}
