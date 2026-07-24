package api

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Settings returns the company-wide configuration.
func (h *Handlers) Settings(c *gin.Context) {
	s, err := h.Store.GetSettings(c.Request.Context())
	if err != nil {
		fail(c, 500, "load settings")
		return
	}
	c.JSON(200, toSettingsDTO(s))
}

// UpdateSettings replaces the company settings (working week + leave-year start).
// A PUT: the whole document is supplied. An empty name falls back to a default,
// matching the web form's behaviour.
func (h *Handlers) UpdateSettings(c *gin.Context) {
	var body SettingsDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "invalid settings body")
		return
	}
	if body.LeaveYearStartMonth < 1 || body.LeaveYearStartMonth > 12 {
		fail(c, 400, "leave_year_start_month must be 1-12")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "My Company"
	}
	if err := h.Store.UpdateSettings(c.Request.Context(), name, body.LeaveYearStartMonth,
		body.WorkMonday, body.WorkTuesday, body.WorkWednesday, body.WorkThursday,
		body.WorkFriday, body.WorkSaturday, body.WorkSunday); err != nil {
		fail(c, 500, "save settings")
		return
	}
	c.Status(204)
}

// createLeaveTypeBody is the JSON body for POST /admin/leave-types.
type createLeaveTypeBody struct {
	Name        string  `json:"name" binding:"required"`
	DefaultDays float64 `json:"default_days"`
	Color       string  `json:"color"`
}

// CreateLeaveType adds a leave type and returns it as a 201.
func (h *Handlers) CreateLeaveType(c *gin.Context) {
	var body createLeaveTypeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "name is required")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		fail(c, 400, "name is required")
		return
	}
	color := body.Color
	if color == "" {
		color = "#6366f1"
	}
	t, err := h.Store.CreateLeaveType(c.Request.Context(), name, body.DefaultDays, color)
	if err != nil {
		fail(c, 500, "create leave type")
		return
	}
	c.JSON(201, toLeaveTypeDTO(t))
}

// ListHolidays returns all public holidays.
func (h *Handlers) ListHolidays(c *gin.Context) {
	rows, err := h.Store.ListHolidays(c.Request.Context())
	if err != nil {
		fail(c, 500, "load holidays")
		return
	}
	out := make([]HolidayDTO, 0, len(rows))
	for _, hol := range rows {
		out = append(out, toHolidayDTO(hol))
	}
	c.JSON(200, out)
}

// createHolidayBody is the JSON body for POST /admin/holidays.
type createHolidayBody struct {
	Name string `json:"name" binding:"required"`
	Date string `json:"date" binding:"required"`
}

// CreateHoliday adds a public holiday and returns it as a 201.
func (h *Handlers) CreateHoliday(c *gin.Context) {
	var body createHolidayBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "name and date are required")
		return
	}
	name := strings.TrimSpace(body.Name)
	date, err := time.Parse(dateLayout, body.Date)
	if name == "" || err != nil {
		fail(c, 400, "name and a valid date (YYYY-MM-DD) are required")
		return
	}
	hol, err := h.Store.CreateHoliday(c.Request.Context(), name, date)
	if err != nil {
		fail(c, 500, "create holiday")
		return
	}
	c.JSON(201, toHolidayDTO(hol))
}

// DeleteHoliday removes a public holiday.
func (h *Handlers) DeleteHoliday(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		fail(c, 400, "bad id")
		return
	}
	if err := h.Store.DeleteHoliday(c.Request.Context(), id); err != nil {
		fail(c, 500, "delete holiday")
		return
	}
	c.Status(204)
}

// setAllocationBody is the JSON body for POST /admin/allocations.
type setAllocationBody struct {
	EmployeeID  int64   `json:"employee_id" binding:"required"`
	LeaveTypeID int64   `json:"leave_type_id" binding:"required"`
	Days        float64 `json:"days"`
}

// SetAllocation upserts an employee's day allocation for a leave type in the
// current leave year.
func (h *Handlers) SetAllocation(c *gin.Context) {
	ctx := c.Request.Context()

	var body setAllocationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, 400, "employee_id and leave_type_id are required")
		return
	}
	year, _, _, err := h.balanceScope(ctx)
	if err != nil {
		fail(c, 500, "load settings")
		return
	}
	if _, err := h.Store.UpsertAllocation(ctx, body.EmployeeID, body.LeaveTypeID, year, body.Days); err != nil {
		fail(c, 500, "save allocation")
		return
	}
	c.Status(204)
}
