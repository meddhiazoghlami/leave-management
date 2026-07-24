package api

import (
	"strings"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/auth"

	"github.com/gin-gonic/gin"
)

// loginRequest is the JSON body for POST /auth/login.
type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// loginResponse is what the client stores after a successful login: the bearer
// token to send on every subsequent request, when it expires, and the caller's
// own profile (so the app can render the home screen without a second call).
type loginResponse struct {
	Token     string  `json:"token"`
	ExpiresAt string  `json:"expires_at"`
	User      UserDTO `json:"user"`
}

// Login verifies credentials and issues a Postgres session token. Unlike the web
// Login (which sets an HttpOnly cookie and redirects), this returns the token in
// the JSON body for the client to hold and send as a bearer token.
func (h *Handlers) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "email and password are required")
		return
	}

	ctx := c.Request.Context()
	emp, err := h.Store.GetEmployeeByEmail(ctx, strings.TrimSpace(req.Email))
	if err != nil || !auth.CheckPassword(emp.PasswordHash, req.Password) {
		// Same message whether the email is unknown or the password is wrong —
		// don't leak which accounts exist.
		fail(c, 401, "invalid email or password")
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		fail(c, 500, "could not start session")
		return
	}
	expires := time.Now().Add(h.Cfg.SessionTTL)
	if _, err := h.Store.CreateSession(ctx, token, emp.ID, expires); err != nil {
		fail(c, 500, "could not persist session")
		return
	}

	c.JSON(200, loginResponse{
		Token:     token,
		ExpiresAt: expires.Format(time.RFC3339),
		User:      toUserDTO(emp),
	})
}

// Logout deletes the session row for the caller's bearer token. It is idempotent:
// a missing or already-deleted token still returns 204.
func (h *Handlers) Logout(c *gin.Context) {
	if token, ok := auth.BearerToken(c); ok {
		_ = h.Store.DeleteSession(c.Request.Context(), token)
	}
	c.Status(204)
}
