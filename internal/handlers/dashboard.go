package handlers

import (
	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/views"

	"github.com/gin-gonic/gin"
)

// Dashboard is the authenticated landing page: the current user's balances and
// a few of their most recent requests.
func (h *Handlers) Dashboard(c *gin.Context) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	balances, err := h.Store.ListBalances(ctx, emp.ID, currentYear())
	if err != nil {
		c.String(500, "load balances: %v", err)
		return
	}
	recent, err := h.Store.ListRequestsByEmployee(ctx, emp.ID)
	if err != nil {
		c.String(500, "load requests: %v", err)
		return
	}
	if len(recent) > 5 {
		recent = recent[:5]
	}

	render(c, 200, views.DashboardPage(views.DashboardData{
		Nav:      h.navFor(c, "dashboard", "Dashboard"),
		Balances: balances,
		Recent:   recent,
	}))
}
