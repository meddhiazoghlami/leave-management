package auth

import (
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

// BearerToken extracts the token from an "Authorization: Bearer <token>" header.
// Mobile clients can't hold the HttpOnly session cookie the web app uses, so the
// JSON API carries the same session token in this header instead.
func BearerToken(c *gin.Context) (string, bool) {
	const prefix = "Bearer "
	h := c.GetHeader("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	return token, token != ""
}

// RequireAPIAuth is the JSON counterpart to RequireAuth: it resolves a bearer
// token (not a cookie) to an employee, and on failure returns a 401 JSON body
// rather than redirecting to /login. It stores the employee under the same
// context key as RequireAuth, so CurrentEmployee / MustEmployee work unchanged
// in API handlers. Sessions are the same Postgres-backed tokens the web app
// issues — a mobile login and a browser login are indistinguishable to the DB.
func RequireAPIAuth(s SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := BearerToken(c)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing or malformed bearer token"})
			return
		}
		emp, err := s.GetSessionEmployee(c.Request.Context(), token)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid or expired token"})
			return
		}
		c.Set(ctxEmployeeKey, emp)
		c.Next()
	}
}

// RequireAPIRole is the JSON counterpart to RequireRole (403 JSON, no redirect).
// It must run after RequireAPIAuth in the chain.
func RequireAPIRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		emp, ok := CurrentEmployee(c)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthenticated"})
			return
		}
		if slices.Contains(roles, emp.Role) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(403, gin.H{"error": "forbidden"})
	}
}
