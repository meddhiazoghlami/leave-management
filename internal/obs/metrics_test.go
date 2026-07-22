package obs_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/meddhiazoghlami/leave-management/internal/obs"

	"github.com/gin-gonic/gin"
)

// A request to a parameterised route must record a series whose `route` label is
// the registered pattern (/items/:id), never the resolved path (/items/42) —
// otherwise every id would spawn its own time series and blow up cardinality.
func TestMiddlewareUsesRoutePatternLabel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(obs.Middleware())
	r.GET("/items/:id", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	r.GET("/metrics", obs.MetricsHandler())

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/items/42", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("handler status = %d, want 200", w.Code)
	}

	body := scrape(t, r)
	want := `http_requests_total{method="GET",route="/items/:id",status="200"}`
	if !strings.Contains(body, want) {
		t.Fatalf("metrics output missing %q\n---\n%s", want, body)
	}
	if strings.Contains(body, "/items/42") {
		t.Fatalf("resolved path leaked into a metric label:\n%s", body)
	}
	if !strings.Contains(body, "http_request_duration_seconds_bucket") {
		t.Fatalf("duration histogram not exported:\n%s", body)
	}
}

// The scrape endpoint itself must not be counted — measuring the measurement is
// noise.
func TestMiddlewareSkipsMetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(obs.Middleware())
	r.GET("/metrics", obs.MetricsHandler())

	body := scrape(t, r)
	if strings.Contains(body, `route="/metrics"`) {
		t.Fatalf("the /metrics endpoint counted itself:\n%s", body)
	}
}

func scrape(t *testing.T, r http.Handler) string {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("scrape status = %d, want 200", w.Code)
	}
	return w.Body.String()
}
