// Package obs holds the app's observability wiring: Prometheus metrics
// (this file), OpenTelemetry tracing (tracing.go), and structured slog logging
// with an optional Loki push handler (logging.go). Keeping all three pillars in
// one package means the middleware, the exporters, and the request logger share
// a single import and are wired together in one place (internal/app).
package obs

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// The RED collectors — Rate (requests total), Errors (status label), Duration
// (histogram). Registered on the default registry via promauto-style MustRegister
// so promhttp.Handler() below picks them up automatically.
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests processed, by method, route and status code.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds, by method, route and status code.",
			Buckets: prometheus.DefBuckets, // .005 … 10s — fine for a web app
		},
		[]string{"method", "route", "status"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being served.",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, httpRequestsInFlight)
}

// Middleware records the RED metrics for every request. It must run early in the
// chain so its timer wraps the real handler work.
//
// The `route` label is c.FullPath() — the registered pattern (e.g.
// "/approvals/:id"), never the resolved path ("/approvals/42"). Using the raw
// URL would explode label cardinality (one time series per id) and eventually
// OOM Prometheus. Requests that match no route report route="<none>".
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// The scrape endpoint measuring itself is noise — skip it.
		if c.Request.URL.Path == metricsPath {
			c.Next()
			return
		}

		httpRequestsInFlight.Inc()
		start := time.Now()

		c.Next()

		httpRequestsInFlight.Dec()

		route := c.FullPath()
		if route == "" {
			route = "<none>"
		}
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		httpRequestsTotal.WithLabelValues(method, route, status).Inc()
		httpRequestDuration.WithLabelValues(method, route, status).Observe(time.Since(start).Seconds())
	}
}

const metricsPath = "/metrics"

// MetricsHandler exposes the Prometheus scrape endpoint (GET /metrics). It is
// unauthenticated — like /healthz — so Prometheus can scrape it without a
// session. Fine for an internal/learning deployment; put it behind the network
// boundary in production.
func MetricsHandler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}
