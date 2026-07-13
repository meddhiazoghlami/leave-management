package handlers

import (
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/views"

	"github.com/gin-gonic/gin"
)

// Employees lists everyone (admin) or just the current manager's reports.
func (h *Handlers) Employees(c *gin.Context) {
	emp := auth.MustEmployee(c)

	scope := emp.ID // manager: only my reports
	if emp.Role == auth.RoleAdmin {
		scope = 0 // admin: everyone
	}
	list, err := h.Store.ListEmployees(c.Request.Context(), scope)
	if err != nil {
		c.String(500, "load employees: %v", err)
		return
	}
	render(c, 200, views.EmployeesPage(views.EmployeesData{
		Nav:       h.navFor(c, "employees", "Team"),
		Employees: list,
	}))
}

// EmployeeProfile shows one employee's balances and requests. Managers may only
// open their own direct reports; admins may open anyone.
func (h *Handlers) EmployeeProfile(c *gin.Context) {
	viewer := auth.MustEmployee(c)
	ctx := c.Request.Context()

	id, ok := idParam(c)
	if !ok {
		c.String(400, "bad id")
		return
	}
	target, err := h.Store.GetEmployee(ctx, id)
	if err != nil {
		c.String(404, "employee not found")
		return
	}

	isReport := target.ManagerID.Valid && target.ManagerID.Int64 == viewer.ID
	if viewer.Role != auth.RoleAdmin && !isReport {
		c.String(403, "not your report")
		return
	}

	year, wStart, wEnd, err := h.balanceScope(ctx)
	if err != nil {
		c.String(500, "load settings: %v", err)
		return
	}
	balances, err := h.Store.ListBalances(ctx, id, year, wStart, wEnd)
	if err != nil {
		c.String(500, "load balances: %v", err)
		return
	}
	requests, err := h.Store.ListRequestsByEmployee(ctx, id)
	if err != nil {
		c.String(500, "load requests: %v", err)
		return
	}
	render(c, 200, views.EmployeeProfilePage(views.EmployeeProfileData{
		Nav:      h.navFor(c, "employees", target.Name),
		Employee: target,
		Balances: balances,
		Requests: requests,
	}))
}
