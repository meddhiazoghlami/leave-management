package api

import (
	"github.com/meddhiazoghlami/leave-management/internal/auth"

	"github.com/gin-gonic/gin"
)

// Employees lists the team directory: everyone for admin/HR, only the caller's
// direct reports for a manager.
func (h *Handlers) Employees(c *gin.Context) {
	emp := auth.MustEmployee(c)

	scope := emp.ID // manager: only my reports
	if auth.IsAdminLevel(emp.Role) {
		scope = 0 // admin/HR: everyone
	}
	rows, err := h.Store.ListEmployees(c.Request.Context(), scope)
	if err != nil {
		fail(c, 500, "load employees")
		return
	}
	out := make([]EmployeeDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEmployeeDTO(r))
	}
	c.JSON(200, out)
}

// employeeProfileResponse bundles a target employee with their balances and
// requests — one call for a mobile profile screen.
type employeeProfileResponse struct {
	Employee UserDTO      `json:"employee"`
	Balances []BalanceDTO `json:"balances"`
	Requests []RequestDTO `json:"requests"`
}

// EmployeeProfile returns one employee's profile, balances and requests. Managers
// may only open their own direct reports; admins and HR may open anyone.
func (h *Handlers) EmployeeProfile(c *gin.Context) {
	viewer := auth.MustEmployee(c)
	ctx := c.Request.Context()

	id, ok := idParam(c)
	if !ok {
		fail(c, 400, "bad id")
		return
	}
	target, err := h.Store.GetEmployee(ctx, id)
	if err != nil {
		fail(c, 404, "employee not found")
		return
	}
	isReport := target.ManagerID.Valid && target.ManagerID.Int64 == viewer.ID
	if !auth.IsAdminLevel(viewer.Role) && !isReport {
		fail(c, 403, "not your report")
		return
	}

	year, wStart, wEnd, err := h.balanceScope(ctx)
	if err != nil {
		fail(c, 500, "load settings")
		return
	}
	balanceRows, err := h.Store.ListBalances(ctx, id, year, wStart, wEnd)
	if err != nil {
		fail(c, 500, "load balances")
		return
	}
	requestRows, err := h.Store.ListRequestsByEmployee(ctx, id)
	if err != nil {
		fail(c, 500, "load requests")
		return
	}

	balances := make([]BalanceDTO, 0, len(balanceRows))
	for _, b := range balanceRows {
		balances = append(balances, toBalanceDTO(b))
	}
	c.JSON(200, employeeProfileResponse{
		Employee: toUserDTO(target),
		Balances: balances,
		Requests: requestDTOs(requestRows),
	})
}
