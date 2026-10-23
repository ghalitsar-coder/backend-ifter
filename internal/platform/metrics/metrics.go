package metrics

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns a private registry so tests and multiple app instances do not
// collide on process-wide Prometheus state.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, route, and status.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by method and route.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
	}

	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.duration,
		m.inFlight,
	)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

// ObserveRequest records one completed request. route must be the registered
// path pattern, never the raw URL, to keep metric cardinality bounded.
func (m *Metrics) ObserveRequest(method, route string, status int, seconds float64) {
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, route).Observe(seconds)
}

func (m *Metrics) IncInFlight() {
	m.inFlight.Inc()
}

func (m *Metrics) DecInFlight() {
	m.inFlight.Dec()
}

// RegisterDBPool exports pgx pool saturation, which is the first signal to
// watch when latency grows under load.
func (m *Metrics) RegisterDBPool(pool *pgxpool.Pool) {
	m.registry.MustRegister(&poolCollector{
		pool: pool,
		connections: prometheus.NewDesc(
			"db_pool_connections",
			"Database pool connections by state.",
			[]string{"state"},
			nil,
		),
		maxConnections: prometheus.NewDesc(
			"db_pool_max_connections",
			"Maximum database pool connections.",
			nil,
			nil,
		),
	})
}

type poolCollector struct {
	pool           *pgxpool.Pool
	connections    *prometheus.Desc
	maxConnections *prometheus.Desc
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connections
	ch <- c.maxConnections
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.maxConnections, prometheus.GaugeValue, float64(stat.MaxConns()))
	for state, value := range map[string]int32{
		"acquired":     stat.AcquiredConns(),
		"idle":         stat.IdleConns(),
		"total":        stat.TotalConns(),
		"constructing": stat.ConstructingConns(),
	} {
		ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(value), state)
	}
}
