package handlers

import (
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/views"

	"github.com/gin-gonic/gin"
)

// Approvals lists the pending requests from the current manager's direct
// reports (admins act as managers too, seeing their own reports).
func (h *Handlers) Approvals(c *gin.Context) {
	emp := auth.MustEmployee(c)
	pending, err := h.Store.ListPendingForManager(c.Request.Context(), emp.ID)
	if err != nil {
		c.String(500, "load approvals: %v", err)
		return
	}
	render(c, 200, views.ApprovalsPage(views.ApprovalsData{
		Nav:     h.navFor(c, "approvals", "Approvals"),
		Pending: pending,
	}))
}

func (h *Handlers) Approve(c *gin.Context) { h.decide(c, "approved", "Request approved.", "success") }
func (h *Handlers) Reject(c *gin.Context)  { h.decide(c, "rejected", "Request rejected.", "error") }

// decide approves or rejects a pending request after confirming the current
// user is allowed to (the requester's manager, or an admin). On success it
// returns an empty body so HTMX removes the card (outerHTML swap) and fires a
// toast.
func (h *Handlers) decide(c *gin.Context, status, msg, kind string) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	id, ok := idParam(c)
	if !ok {
		c.String(400, "bad id")
		return
	}

	req, err := h.Store.GetLeaveRequest(ctx, id)
	if err != nil {
		c.String(404, "request not found")
		return
	}
	if !h.canDecide(c, emp, req.EmployeeID) {
		c.String(403, "not your report")
		return
	}
	if err := h.Store.SetRequestStatus(ctx, id, status, emp.ID); err != nil {
		c.String(500, "decide: %v", err)
		return
	}

	toast(c, msg, kind)
	c.Status(200) // empty body -> HTMX swaps the card out
}

// canDecide reports whether decider may approve/reject a request owned by
// requesterID: admins and HR can decide anything; managers only their direct
// reports.
func (h *Handlers) canDecide(c *gin.Context, decider db.Employee, requesterID int64) bool {
	if auth.IsAdminLevel(decider.Role) {
		return true
	}
	requester, err := h.Store.GetEmployee(c.Request.Context(), requesterID)
	if err != nil {
		return false
	}
	return requester.ManagerID.Valid && requester.ManagerID.Int64 == decider.ID
}
