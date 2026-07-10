package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dzovi/leave-management/internal/config"
	"github.com/dzovi/leave-management/internal/handlers"
	"github.com/dzovi/leave-management/internal/server"
	"github.com/gin-gonic/gin"
)

// TestUnauthenticatedRedirect exercises the real router + RequireAuth: a request
// with no session cookie is bounced to /login before any handler (or the DB) is
// touched — which is why this test needs no database (nil store is safe here).
func TestUnauthenticatedRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := handlers.New(nil, config.Config{})
	r := server.New(h, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want %q", loc, "/login")
	}
}
