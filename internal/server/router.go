// Package server wires routes to handlers and layers on the auth middleware.
// Keeping this out of main.go keeps the entrypoint tiny and the routing table
// readable in one place.
package server

import (
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/handlers"

	"github.com/gin-gonic/gin"
)

// New builds the Gin engine. The session store is needed by the RequireAuth
// middleware to resolve session cookies; the handlers already carry their own
// store. Both are satisfied by the concrete *store.Store at wire time.
func New(h *handlers.Handlers, s auth.SessionStore) *gin.Engine {
	r := gin.Default()

	// Vite build output (Phase 8). Unused in dev, where assets come from :5173.
	r.Static("/build", "./public/build")

	// Readiness probe for containers/orchestrators — no session required.
	r.GET("/healthz", h.Health)

	// Public routes — no session required.
	r.GET("/login", h.ShowLogin)
	r.POST("/login", h.Login)

	// Everything below requires a valid session.
	app := r.Group("/", auth.RequireAuth(s))
	{
		app.POST("/logout", h.Logout)
		app.GET("/", h.Dashboard)

		app.GET("/requests", h.Requests)
		app.POST("/requests", h.CreateRequest)
		app.POST("/requests/:id/cancel", h.CancelRequest)

		app.GET("/calendar", h.Calendar)
		app.GET("/calendar/month", h.CalendarMonthFragment)
	}

	// Manager + admin: approvals and the team directory.
	mgr := app.Group("/", auth.RequireRole(auth.RoleManager, auth.RoleAdmin))
	{
		mgr.GET("/approvals", h.Approvals)
		mgr.POST("/approvals/:id/approve", h.Approve)
		mgr.POST("/approvals/:id/reject", h.Reject)
		mgr.GET("/employees", h.Employees)
		mgr.GET("/employees/:id", h.EmployeeProfile)
	}

	// Admin only: leave types, holidays, allocations.
	adm := app.Group("/admin", auth.RequireRole(auth.RoleAdmin))
	{
		adm.GET("", h.Admin)
		adm.POST("/leave-types", h.CreateLeaveType)
		adm.POST("/holidays", h.CreateHoliday)
		adm.POST("/holidays/:id/delete", h.DeleteHoliday)
		adm.POST("/allocations", h.SetAllocation)
	}

	return r
}
