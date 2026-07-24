package api

import (
	"strings"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/auth"
	"github.com/meddhiazoghlami/leave-management/internal/db"
	"github.com/meddhiazoghlami/leave-management/internal/leave"

	"github.com/gin-gonic/gin"
)

// Me returns the authenticated caller's own profile.
func (h *Handlers) Me(c *gin.Context) {
	c.JSON(200, toUserDTO(auth.MustEmployee(c)))
}

// MyBalances returns the caller's allocation/usage/remaining per leave type for
// the current leave year.
func (h *Handlers) MyBalances(c *gin.Context) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	year, wStart, wEnd, err := h.balanceScope(ctx)
	if err != nil {
		fail(c, 500, "load settings")
		return
	}
	rows, err := h.Store.ListBalances(ctx, emp.ID, year, wStart, wEnd)
	if err != nil {
		fail(c, 500, "load balances")
		return
	}
	out := make([]BalanceDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toBalanceDTO(r))
	}
	c.JSON(200, out)
}

// LeaveTypes lists the configurable leave types (needed to populate a submit
// form on the client).
func (h *Handlers) LeaveTypes(c *gin.Context) {
	rows, err := h.Store.ListLeaveTypes(c.Request.Context())
	if err != nil {
		fail(c, 500, "load leave types")
		return
	}
	out := make([]LeaveTypeDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, toLeaveTypeDTO(t))
	}
	c.JSON(200, out)
}

// MyRequests lists the caller's own leave requests, newest first (ordering is in
// the SQL).
func (h *Handlers) MyRequests(c *gin.Context) {
	emp := auth.MustEmployee(c)
	rows, err := h.Store.ListRequestsByEmployee(c.Request.Context(), emp.ID)
	if err != nil {
		fail(c, 500, "load requests")
		return
	}
	c.JSON(200, requestDTOs(rows))
}

// createRequestBody is the JSON body for POST /requests. Dates are date-only
// strings (YYYY-MM-DD); reason is optional.
type createRequestBody struct {
	LeaveTypeID int64  `json:"leave_type_id" binding:"required"`
	StartDate   string `json:"start_date" binding:"required"`
	EndDate     string `json:"end_date" binding:"required"`
	Reason      string `json:"reason"`
}

// CreateRequest validates the body, computes working days (excluding weekends
// and public holidays per company settings), inserts the request, and returns
// the created request as a 201. This mirrors the web CreateRequest's rules
// exactly — same working-day math, same guards.
func (h *Handlers) CreateRequest(c *gin.Context) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	var body createRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "leave_type_id, start_date and end_date are required")
		return
	}
	start, err1 := time.Parse(dateLayout, body.StartDate)
	end, err2 := time.Parse(dateLayout, body.EndDate)
	if err1 != nil || err2 != nil {
		fail(c, 400, "dates must be YYYY-MM-DD")
		return
	}
	if end.Before(start) {
		fail(c, 400, "end date can't be before the start date")
		return
	}

	// Working days = working weekdays in range minus any public holidays in that
	// window. The working week comes from company settings (config over code).
	settings, err := h.Store.GetSettings(ctx)
	if err != nil {
		fail(c, 500, "load settings")
		return
	}
	holidays, err := h.Store.ListHolidaysInRange(ctx, start, end)
	if err != nil {
		fail(c, 500, "load holidays")
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
		fail(c, 400, "that range has no working days (weekends/holidays only)")
		return
	}

	reason := strings.TrimSpace(body.Reason)
	created, err := h.Store.CreateLeaveRequest(ctx, emp.ID, body.LeaveTypeID, start, end, float64(workingDays), reason)
	if err != nil {
		fail(c, 500, "create request")
		return
	}

	c.JSON(201, gin.H{
		"id":            created.ID,
		"leave_type_id": created.LeaveTypeID,
		"start_date":    created.StartDate.Format(dateLayout),
		"end_date":      created.EndDate.Format(dateLayout),
		"working_days":  created.WorkingDays,
		"reason":        created.Reason,
		"status":        created.Status,
		"created_at":    created.CreatedAt.Format(time.RFC3339),
	})
}

// CancelRequest cancels one of the caller's own pending requests. The ownership +
// pending guard is in the SQL, so a request that isn't the caller's (or isn't
// pending) simply affects no rows.
func (h *Handlers) CancelRequest(c *gin.Context) {
	emp := auth.MustEmployee(c)
	ctx := c.Request.Context()

	id, ok := idParam(c)
	if !ok {
		fail(c, 400, "bad id")
		return
	}
	if err := h.Store.CancelOwnRequest(ctx, id, emp.ID); err != nil {
		fail(c, 500, "cancel request")
		return
	}
	c.Status(204)
}

// Calendar returns approved leave overlapping a [start, end] range so a client
// can paint its own calendar. Range defaults to the current month when the query
// params are absent or invalid.
func (h *Handlers) Calendar(c *gin.Context) {
	ctx := c.Request.Context()
	now := time.Now()

	start, err1 := time.Parse(dateLayout, c.Query("start"))
	end, err2 := time.Parse(dateLayout, c.Query("end"))
	if err1 != nil || err2 != nil {
		// Default to the current calendar month.
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, -1)
	}
	if end.Before(start) {
		fail(c, 400, "end must be on or after start")
		return
	}

	rows, err := h.Store.ListApprovedInRange(ctx, start, end)
	if err != nil {
		fail(c, 500, "load calendar")
		return
	}
	out := make([]CalendarEntryDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCalendarEntryDTO(r))
	}
	c.JSON(200, out)
}

// requestDTOs maps a slice of request rows to DTOs (shared by MyRequests and the
// employee-profile endpoint).
func requestDTOs(rows []db.ListRequestsByEmployeeRow) []RequestDTO {
	out := make([]RequestDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRequestDTO(r))
	}
	return out
}
