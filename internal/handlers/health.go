package handlers

import "github.com/gin-gonic/gin"

// Health is an unauthenticated readiness probe (GET /healthz): 200 when the
// database is reachable, 503 otherwise. Used by the container HEALTHCHECK and
// by any orchestrator's liveness/readiness checks.
func (h *Handlers) Health(c *gin.Context) {
	if err := h.Store.Ping(c.Request.Context()); err != nil {
		c.String(503, "unavailable")
		return
	}
	c.String(200, "ok")
}
