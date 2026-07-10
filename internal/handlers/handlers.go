// Package handlers holds the HTTP layer: one method per route, grouped by
// feature across several files. Handlers depend only on the store (data) and
// views (HTML); auth/session concerns live in package auth.
package handlers

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/dzovi/leave-management/internal/auth"
	"github.com/dzovi/leave-management/internal/config"
	"github.com/dzovi/leave-management/internal/store"
	"github.com/dzovi/leave-management/views"
	"github.com/gin-gonic/gin"
)

// Handlers bundles the dependencies every route needs.
type Handlers struct {
	Store *store.Store
	Cfg   config.Config
}

func New(s *store.Store, cfg config.Config) *Handlers {
	return &Handlers{Store: s, Cfg: cfg}
}

// render writes a templ component as an HTML response with the given status.
func render(c *gin.Context, status int, comp templ.Component) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = comp.Render(c.Request.Context(), c.Writer)
}

// toast sets the HX-Trigger header so the Alpine toast host (in the layout)
// pops a message. kind is "success" or "error" (controls the colour).
func toast(c *gin.Context, message, kind string) {
	payload := map[string]any{"toast": map[string]string{"message": message, "type": kind}}
	b, _ := json.Marshal(payload)
	c.Header("HX-Trigger", string(b))
}

// navFor builds the header view model for the current (authenticated) user,
// including the manager pending-approval badge count.
func (h *Handlers) navFor(c *gin.Context, active, title string) views.Nav {
	emp := auth.MustEmployee(c)
	nav := views.Nav{Title: title, Name: emp.Name, Role: emp.Role, Active: active}
	if nav.IsManager() {
		if n, err := h.Store.CountPendingForManager(c.Request.Context(), emp.ID); err == nil {
			nav.PendingCount = n
		}
	}
	return nav
}

// currentYear is the year balances/allocations are scoped to.
func currentYear() int32 { return int32(time.Now().Year()) }

// idParam parses a numeric ":id" route param.
func idParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	return id, err == nil
}
