// Package server wires routes to handlers and layers on the auth middleware.
// Keeping this out of main.go keeps the entrypoint tiny and the routing table
// readable in one place.
package server

import (
	"log/slog"

	"github.com/meddhiazoghlami/leave-management/internal/api"
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/config"
	"github.com/meddhiazoghlami/leave-management/internal/handlers"
	"github.com/meddhiazoghlami/leave-management/internal/obs"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// New builds the Gin engine. The session store is needed by the RequireAuth
// middleware to resolve session cookies; the handlers already carry their own
// store. Both are satisfied by the concrete *store.Store at wire time. cfg,
// logger and tp are the observability collaborators (internal/obs).
func New(h *handlers.Handlers, apiH *api.Handlers, s auth.SessionStore, cfg config.Config, logger *slog.Logger, tp trace.TracerProvider) *gin.Engine {
	// gin.New() (not gin.Default()) so we own the middleware chain. Order is
	// deliberate:
	//   1. Recovery — outermost, so a panic anywhere still returns 500.
	//   2. otelgin — establishes the trace span on the request context. It must
	//      sit OUTSIDE RequestLogger: otelgin restores the span-less context in a
	//      deferred call as it unwinds, so any logging done after c.Next() only
	//      sees the trace_id if it runs *inside* otelgin (unwinds first).
	//   3. RequestLogger — structured slog line per request, with trace_id.
	//   4. Metrics — RED counters/histogram.
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(otelgin.Middleware(cfg.ServiceName, otelgin.WithTracerProvider(tp)))
	r.Use(obs.RequestLogger(logger))
	r.Use(obs.Middleware())

	// Vite build output (Phase 8). Unused in dev, where assets come from :5173.
	r.Static("/build", "./public/build")

	// Prometheus scrape endpoint — unauthenticated, like /healthz below.
	r.GET("/metrics", obs.MetricsHandler())

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

	// Manager + admin + HR: approvals and the team directory.
	mgr := app.Group("/", auth.RequireRole(auth.RoleManager, auth.RoleAdmin, auth.RoleHR))
	{
		mgr.GET("/approvals", h.Approvals)
		mgr.POST("/approvals/:id/approve", h.Approve)
		mgr.POST("/approvals/:id/reject", h.Reject)
		mgr.GET("/employees", h.Employees)
		mgr.GET("/employees/:id", h.EmployeeProfile)
	}

	// Admin + HR: general settings, leave types, holidays, allocations.
	adm := app.Group("/admin", auth.RequireRole(auth.RoleAdmin, auth.RoleHR))
	{
		adm.GET("", h.Admin)
		adm.POST("/settings", h.SaveSettings)
		adm.POST("/leave-types", h.CreateLeaveType)
		adm.POST("/holidays", h.CreateHoliday)
		adm.POST("/holidays/:id/delete", h.DeleteHoliday)
		adm.POST("/allocations", h.SetAllocation)
	}

	mountAPI(r, apiH, s)

	return r
}

// mountAPI hangs the JSON REST API off /api/v1. It is a wholly separate tree
// from the HTML routes above: it authenticates with a bearer token (not the
// session cookie) and returns JSON on every path, including auth failures. The
// data layer and business rules are shared — only the transport differs. This is
// what a mobile client integrates against.
func mountAPI(r *gin.Engine, h *api.Handlers, s auth.SessionStore) {
	v1 := r.Group("/api/v1")

	// Public: obtain a token.
	v1.POST("/auth/login", h.Login)

	// Everything below requires a valid bearer token.
	auth1 := v1.Group("/", auth.RequireAPIAuth(s))
	{
		auth1.POST("/auth/logout", h.Logout)

		auth1.GET("/me", h.Me)
		auth1.GET("/me/balances", h.MyBalances)
		auth1.GET("/leave-types", h.LeaveTypes)
		auth1.GET("/calendar", h.Calendar)

		auth1.GET("/requests", h.MyRequests)
		auth1.POST("/requests", h.CreateRequest)
		auth1.POST("/requests/:id/cancel", h.CancelRequest)
	}

	// Manager + admin + HR: approvals and the team directory.
	mgr := auth1.Group("/", auth.RequireAPIRole(auth.RoleManager, auth.RoleAdmin, auth.RoleHR))
	{
		mgr.GET("/approvals", h.Approvals)
		mgr.POST("/approvals/:id/approve", h.Approve)
		mgr.POST("/approvals/:id/reject", h.Reject)
		mgr.GET("/employees", h.Employees)
		mgr.GET("/employees/:id", h.EmployeeProfile)
	}

	// Admin + HR: settings, leave types, holidays, allocations.
	adm := auth1.Group("/admin", auth.RequireAPIRole(auth.RoleAdmin, auth.RoleHR))
	{
		adm.GET("/settings", h.Settings)
		adm.PUT("/settings", h.UpdateSettings)
		adm.POST("/leave-types", h.CreateLeaveType)
		adm.GET("/holidays", h.ListHolidays)
		adm.POST("/holidays", h.CreateHoliday)
		adm.DELETE("/holidays/:id", h.DeleteHoliday)
		adm.POST("/allocations", h.SetAllocation)
	}
}
