package auth

import (
	"context"
	"slices"

	"github.com/meddhiazoghlami/leave-management/internal/db"

	"github.com/gin-gonic/gin"
)

// SessionStore is the slice of the data layer RequireAuth needs: resolve a
// session token to its employee. Declared here (consumer-side) so the middleware
// depends on a one-method interface rather than the concrete *store.Store, which
// keeps it unit-testable with a fake and avoids an import of package store.
type SessionStore interface {
	GetSessionEmployee(ctx context.Context, token string) (db.Employee, error)
}

// Role values stored in employees.role.
const (
	RoleEmployee = "employee"
	RoleManager  = "manager"
	RoleAdmin    = "admin"
)

// ctxEmployeeKey is where RequireAuth stashes the resolved employee on the Gin
// context.
const ctxEmployeeKey = "currentEmployee"

// CurrentEmployee returns the employee attached by RequireAuth, or ok=false if
// the request isn't authenticated (i.e. RequireAuth didn't run / didn't match).
func CurrentEmployee(c *gin.Context) (db.Employee, bool) {
	v, ok := c.Get(ctxEmployeeKey)
	if !ok {
		return db.Employee{}, false
	}
	e, ok := v.(db.Employee)
	return e, ok
}

// MustEmployee is CurrentEmployee for handlers already behind RequireAuth, where
// the employee is guaranteed present.
func MustEmployee(c *gin.Context) db.Employee {
	e, _ := CurrentEmployee(c)
	return e
}

// RequireAuth resolves the session cookie to an employee and stores it on the
// context, or bounces to /login. It short-circuits (no DB call) when there's no
// cookie — which keeps the unauthenticated path cheap and testable.
func RequireAuth(s SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(CookieName)
		if err != nil || token == "" {
			redirectToLogin(c)
			c.Abort()
			return
		}
		emp, err := s.GetSessionEmployee(c.Request.Context(), token)
		if err != nil {
			// Expired or bogus token: clear the stale cookie and re-auth.
			ClearSessionCookie(c)
			redirectToLogin(c)
			c.Abort()
			return
		}
		c.Set(ctxEmployeeKey, emp)
		c.Next()
	}
}

// RequireRole gates a route to one of the given roles. It must run after
// RequireAuth in the middleware chain.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		emp, ok := CurrentEmployee(c)
		if !ok {
			redirectToLogin(c)
			c.Abort()
			return
		}
		if slices.Contains(roles, emp.Role) {
			c.Next()
			return
		}
		c.String(403, "403 forbidden")
		c.Abort()
	}
}

// redirectToLogin sends the browser to /login. For HTMX requests a normal 302
// would only swap the login page into a fragment, so we use HX-Redirect to make
// the whole page navigate instead.
func redirectToLogin(c *gin.Context) {
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/login")
		c.Status(200)
		return
	}
	c.Redirect(302, "/login")
}
