package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/db"

	"github.com/gin-gonic/gin"
)

// fakeSessionStore is an auth.SessionStore whose behaviour the test controls.
type fakeSessionStore struct {
	emp db.Employee
	err error
}

func (f fakeSessionStore) GetSessionEmployee(_ context.Context, _ string) (db.Employee, error) {
	return f.emp, f.err
}

// terminal is the handler that runs iff the middleware chain calls Next; it
// echoes the resolved employee so tests can assert the context was populated.
func terminal(c *gin.Context) {
	emp := auth.MustEmployee(c)
	c.String(http.StatusOK, "ok:%d:%s", emp.ID, emp.Role)
}

func TestRequireAuth_NoCookieRedirects(t *testing.T) {
	r := gin.New()
	r.GET("/", auth.RequireAuth(fakeSessionStore{}), terminal)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location = %q, want /login", loc)
	}
}

func TestRequireAuth_NoCookieHTMXRedirect(t *testing.T) {
	r := gin.New()
	r.GET("/", auth.RequireAuth(fakeSessionStore{}), terminal)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// HTMX gets a 200 + HX-Redirect header so the whole page navigates.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for HTMX redirect", w.Code)
	}
	if hx := w.Header().Get("HX-Redirect"); hx != "/login" {
		t.Fatalf("HX-Redirect = %q, want /login", hx)
	}
}

func TestRequireAuth_BadTokenClearsAndRedirects(t *testing.T) {
	store := fakeSessionStore{err: errors.New("no such session")}
	r := gin.New()
	r.GET("/", auth.RequireAuth(store), terminal)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "stale"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	// The stale cookie is cleared (expired) on the way out.
	var cleared bool
	for _, ck := range w.Result().Cookies() {
		if ck.Name == auth.CookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("expected the stale session cookie to be cleared")
	}
}

func TestRequireAuth_ValidTokenCallsNext(t *testing.T) {
	store := fakeSessionStore{emp: db.Employee{ID: 42, Role: auth.RoleManager}}
	r := gin.New()
	r.GET("/", auth.RequireAuth(store), terminal)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: "good"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "ok:42:manager" {
		t.Fatalf("status %d body %q, want 200 ok:42:manager", w.Code, w.Body.String())
	}
}

func TestRequireRole(t *testing.T) {
	// setEmployee stands in for RequireAuth, seeding the context.
	setEmployee := func(emp db.Employee, present bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			if present {
				c.Set("currentEmployee", emp)
			}
			c.Next()
		}
	}

	tests := []struct {
		name     string
		present  bool
		role     string
		allow    []string
		wantCode int
	}{
		{"admin allowed", true, auth.RoleAdmin, []string{auth.RoleManager, auth.RoleAdmin}, http.StatusOK},
		{"manager allowed", true, auth.RoleManager, []string{auth.RoleManager, auth.RoleAdmin}, http.StatusOK},
		{"employee forbidden", true, auth.RoleEmployee, []string{auth.RoleManager, auth.RoleAdmin}, http.StatusForbidden},
		{"no employee redirects", false, "", []string{auth.RoleAdmin}, http.StatusFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/", setEmployee(db.Employee{ID: 1, Role: tc.role}, tc.present),
				auth.RequireRole(tc.allow...), terminal)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantCode)
			}
		})
	}
}

func TestCurrentEmployee(t *testing.T) {
	// Absent: ok == false.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if _, ok := auth.CurrentEmployee(c); ok {
		t.Error("CurrentEmployee ok = true on empty context")
	}

	// Wrong type stored under the key: ok == false (defensive type assertion).
	c.Set("currentEmployee", "not-an-employee")
	if _, ok := auth.CurrentEmployee(c); ok {
		t.Error("CurrentEmployee ok = true for wrong stored type")
	}

	// Present: returns the employee.
	c.Set("currentEmployee", db.Employee{ID: 7, Role: auth.RoleEmployee})
	got, ok := auth.CurrentEmployee(c)
	if !ok || got.ID != 7 {
		t.Fatalf("CurrentEmployee = %+v ok %v, want id 7 ok true", got, ok)
	}

	// MustEmployee returns the zero value when absent rather than panicking.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	if emp := auth.MustEmployee(c2); emp.ID != 0 {
		t.Errorf("MustEmployee on empty context = %+v, want zero", emp)
	}
}
