package api

import (
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/db"

	"github.com/gin-gonic/gin"
)

// Approvals lists pending requests from the caller's direct reports (admins and
// HR act as managers over everyone via the SQL scoping).
func (h *Handlers) Approvals(c *gin.Context) {
	emp := auth.MustEmployee(c)
	rows, err := h.Store.ListPendingForManager(c.Request.Context(), emp.ID)
	if err != nil {
		fail(c, 500, "load approvals")
		return
	}
	out := make([]PendingDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPendingDTO(r))
	}
	c.JSON(200, out)
}

// Approve and Reject decide a pending request; both delegate to decide.
func (h *Handlers) Approve(c *gin.Context) { h.decide(c, "approved") }
func (h *Handlers) Reject(c *gin.Context)  { h.decide(c, "rejected") }

// decide sets a request's status after confirming the caller may act on it (the
// requester's manager, or an admin/HR). Returns the request's new status on
// success — same authorization rules as the web decide handler.
func (h *Handlers) decide(c *gin.Context, status string) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	id, ok := idParam(c)
	if !ok {
		fail(c, 400, "bad id")
		return
	}
	req, err := h.Store.GetLeaveRequest(ctx, id)
	if err != nil {
		fail(c, 404, "request not found")
		return
	}
	if !h.canDecide(c, emp, req.EmployeeID) {
		fail(c, 403, "not your report")
		return
	}
	if err := h.Store.SetRequestStatus(ctx, id, status, emp.ID); err != nil {
		fail(c, 500, "decide request")
		return
	}
	c.JSON(200, gin.H{"id": id, "status": status})
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
