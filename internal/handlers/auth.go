package handlers

import (
	"strings"
	"time"

	"github.com/dzovi/leave-management/internal/auth"
	"github.com/dzovi/leave-management/views"
	"github.com/gin-gonic/gin"
)

// ShowLogin renders the standalone login page.
func (h *Handlers) ShowLogin(c *gin.Context) {
	render(c, 200, views.LoginPage("", ""))
}

// Login verifies credentials, creates a Postgres session, and drops the token
// in an HttpOnly cookie. On failure it re-renders the form with the email kept.
func (h *Handlers) Login(c *gin.Context) {
	email := strings.TrimSpace(c.PostForm("email"))
	password := c.PostForm("password")

	emp, err := h.Store.GetEmployeeByEmail(c.Request.Context(), email)
	if err != nil || !auth.CheckPassword(emp.PasswordHash, password) {
		// Same message whether the email is unknown or the password is wrong —
		// don't leak which accounts exist.
		render(c, 200, views.LoginPage(email, "Invalid email or password."))
		return
	}

	token, err := auth.NewToken()
	if err != nil {
		c.String(500, "could not start session")
		return
	}
	expires := time.Now().Add(h.Cfg.SessionTTL)
	if _, err := h.Store.CreateSession(c.Request.Context(), token, emp.ID, expires); err != nil {
		c.String(500, "could not persist session")
		return
	}
	auth.SetSessionCookie(c, token, h.Cfg.SessionTTL)
	c.Redirect(303, "/")
}

// Logout deletes the session row and clears the cookie.
func (h *Handlers) Logout(c *gin.Context) {
	if token, err := c.Cookie(auth.CookieName); err == nil && token != "" {
		_ = h.Store.DeleteSession(c.Request.Context(), token)
	}
	auth.ClearSessionCookie(c)
	c.Redirect(303, "/login")
}
