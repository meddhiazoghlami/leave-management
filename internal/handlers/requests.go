package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/leave"
	"github.com/meddhiazoghlami/leave-management/views"

	"github.com/gin-gonic/gin"
)

// Requests renders the employee's own request list plus the submit modal.
func (h *Handlers) Requests(c *gin.Context) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	requests, err := h.Store.ListRequestsByEmployee(ctx, emp.ID)
	if err != nil {
		c.String(500, "load requests: %v", err)
		return
	}
	types, err := h.Store.ListLeaveTypes(ctx)
	if err != nil {
		c.String(500, "load leave types: %v", err)
		return
	}

	render(c, 200, views.RequestsPage(views.RequestsData{
		Nav:        h.navFor(c, "requests", "My Requests"),
		Requests:   requests,
		LeaveTypes: types,
		Today:      time.Now().Format("2006-01-02"),
	}))
}

// CreateRequest validates the form, computes working days (excluding weekends
// and public holidays), inserts the request, and returns the refreshed list
// fragment + a toast. Validation failures return 400 with an error toast so the
// modal stays open with the user's input intact.
func (h *Handlers) CreateRequest(c *gin.Context) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	typeID, err := strconv.ParseInt(c.PostForm("leave_type_id"), 10, 64)
	if err != nil {
		toast(c, "Please choose a leave type.", "error")
		c.Status(400)
		return
	}
	start, err1 := time.Parse("2006-01-02", c.PostForm("start_date"))
	end, err2 := time.Parse("2006-01-02", c.PostForm("end_date"))
	if err1 != nil || err2 != nil {
		toast(c, "Please provide valid dates.", "error")
		c.Status(400)
		return
	}
	if end.Before(start) {
		toast(c, "End date can't be before the start date.", "error")
		c.Status(400)
		return
	}

	// Working days = working weekdays in range minus any public holidays in that
	// window. The working week comes from company settings now (config over
	// code), so a Fri/Sat weekend is honoured here.
	settings, err := h.Store.GetSettings(ctx)
	if err != nil {
		c.String(500, "load settings: %v", err)
		return
	}
	holidays, err := h.Store.ListHolidaysInRange(ctx, start, end)
	if err != nil {
		c.String(500, "load holidays: %v", err)
		return
	}
	dates := make([]time.Time, 0, len(holidays))
	for _, hol := range holidays {
		dates = append(dates, hol.HolidayDate)
	}
	week := leave.WorkingWeek(
		settings.WorkMonday, settings.WorkTuesday, settings.WorkWednesday,
		settings.WorkThursday, settings.WorkFriday, settings.WorkSaturday, settings.WorkSunday,
	)
	workingDays := leave.WorkingDays(start, end, week, leave.HolidaySet(dates))
	if workingDays == 0 {
		toast(c, "That range has no working days (weekends/holidays only).", "error")
		c.Status(400)
		return
	}

	reason := strings.TrimSpace(c.PostForm("reason"))
	if _, err := h.Store.CreateLeaveRequest(ctx, emp.ID, typeID, start, end, float64(workingDays), reason); err != nil {
		c.String(500, "create request: %v", err)
		return
	}

	requests, err := h.Store.ListRequestsByEmployee(ctx, emp.ID)
	if err != nil {
		c.String(500, "load requests: %v", err)
		return
	}
	toast(c, "Leave request submitted.", "success")
	render(c, 200, views.RequestListRows(requests))
}

// CancelRequest cancels one of the current user's own pending requests, then
// returns the refreshed list. The ownership + pending guard is in the SQL.
func (h *Handlers) CancelRequest(c *gin.Context) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	id, ok := idParam(c)
	if !ok {
		c.String(400, "bad id")
		return
	}
	if err := h.Store.CancelOwnRequest(ctx, id, emp.ID); err != nil {
		c.String(500, "cancel: %v", err)
		return
	}

	requests, err := h.Store.ListRequestsByEmployee(ctx, emp.ID)
	if err != nil {
		c.String(500, "load requests: %v", err)
		return
	}
	toast(c, "Request cancelled.", "success")
	render(c, 200, views.RequestListRows(requests))
}
