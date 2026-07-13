package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/meddhiazoghlami/leave-management/internal/leave"
	"github.com/meddhiazoghlami/leave-management/views"

	"github.com/gin-gonic/gin"
)

// Admin renders the single admin page: general settings, leave types, holidays,
// and allocations.
func (h *Handlers) Admin(c *gin.Context) {
	ctx := c.Request.Context()

	types, err := h.Store.ListLeaveTypes(ctx)
	if err != nil {
		c.String(500, "load leave types: %v", err)
		return
	}
	holidays, err := h.Store.ListHolidays(ctx)
	if err != nil {
		c.String(500, "load holidays: %v", err)
		return
	}
	employees, err := h.Store.ListEmployees(ctx, 0)
	if err != nil {
		c.String(500, "load employees: %v", err)
		return
	}
	settings, err := h.Store.GetSettings(ctx)
	if err != nil {
		c.String(500, "load settings: %v", err)
		return
	}
	// The allocation section is scoped to the current leave year's label so it
	// lines up with the balances shown on the dashboard.
	_, _, label := leave.LeaveYearWindow(time.Now(), int(settings.LeaveYearStartMonth))
	render(c, 200, views.AdminPage(views.AdminData{
		Nav:        h.navFor(c, "admin", "Admin"),
		LeaveTypes: types,
		Holidays:   holidays,
		Employees:  employees,
		Settings:   settings,
		Year:       label,
	}))
}

// SaveSettings persists the general company settings (working week + leave-year
// start month). Uses hx-swap="none": it just saves and fires a toast.
func (h *Handlers) SaveSettings(c *gin.Context) {
	ctx := c.Request.Context()

	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = "My Company"
	}
	month, err := strconv.Atoi(c.PostForm("leave_year_start_month"))
	if err != nil || month < 1 || month > 12 {
		toast(c, "Pick a valid leave-year start month.", "error")
		c.Status(400)
		return
	}
	// Unchecked checkboxes aren't submitted at all, so presence == working day.
	on := func(field string) bool { return c.PostForm(field) != "" }
	if err := h.Store.UpdateSettings(ctx, name, int32(month),
		on("work_monday"), on("work_tuesday"), on("work_wednesday"), on("work_thursday"),
		on("work_friday"), on("work_saturday"), on("work_sunday")); err != nil {
		c.String(500, "save settings: %v", err)
		return
	}
	toast(c, "Settings saved.", "success")
	c.Status(200)
}

// CreateLeaveType adds a leave type and returns the refreshed list fragment.
func (h *Handlers) CreateLeaveType(c *gin.Context) {
	ctx := c.Request.Context()

	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		toast(c, "Name is required.", "error")
		c.Status(400)
		return
	}
	days, _ := strconv.ParseFloat(c.PostForm("default_days"), 64)
	color := c.PostForm("color")
	if color == "" {
		color = "#6366f1"
	}
	if _, err := h.Store.CreateLeaveType(ctx, name, days, color); err != nil {
		c.String(500, "create leave type: %v", err)
		return
	}

	types, err := h.Store.ListLeaveTypes(ctx)
	if err != nil {
		c.String(500, "load leave types: %v", err)
		return
	}
	toast(c, "Leave type added.", "success")
	render(c, 200, views.LeaveTypeRows(types))
}

// CreateHoliday adds a public holiday and returns the refreshed list fragment.
func (h *Handlers) CreateHoliday(c *gin.Context) {
	ctx := c.Request.Context()

	name := strings.TrimSpace(c.PostForm("name"))
	date, err := time.Parse("2006-01-02", c.PostForm("holiday_date"))
	if name == "" || err != nil {
		toast(c, "Name and a valid date are required.", "error")
		c.Status(400)
		return
	}
	if _, err := h.Store.CreateHoliday(ctx, name, date); err != nil {
		c.String(500, "create holiday: %v", err)
		return
	}

	holidays, err := h.Store.ListHolidays(ctx)
	if err != nil {
		c.String(500, "load holidays: %v", err)
		return
	}
	toast(c, "Holiday added.", "success")
	render(c, 200, views.HolidayRows(holidays))
}

// DeleteHoliday removes a holiday and returns the refreshed list fragment.
func (h *Handlers) DeleteHoliday(c *gin.Context) {
	ctx := c.Request.Context()

	id, ok := idParam(c)
	if !ok {
		c.String(400, "bad id")
		return
	}
	if err := h.Store.DeleteHoliday(ctx, id); err != nil {
		c.String(500, "delete holiday: %v", err)
		return
	}

	holidays, err := h.Store.ListHolidays(ctx)
	if err != nil {
		c.String(500, "load holidays: %v", err)
		return
	}
	toast(c, "Holiday removed.", "success")
	render(c, 200, views.HolidayRows(holidays))
}

// SetAllocation upserts an employee's day allocation for a leave type this year.
// The form uses hx-swap="none", so we just persist and fire a toast.
func (h *Handlers) SetAllocation(c *gin.Context) {
	ctx := c.Request.Context()

	empID, err1 := strconv.ParseInt(c.PostForm("employee_id"), 10, 64)
	typeID, err2 := strconv.ParseInt(c.PostForm("leave_type_id"), 10, 64)
	days, err3 := strconv.ParseFloat(c.PostForm("days"), 64)
	if err1 != nil || err2 != nil || err3 != nil {
		toast(c, "Please fill in all allocation fields.", "error")
		c.Status(400)
		return
	}
	year, _, _, err := h.balanceScope(ctx)
	if err != nil {
		c.String(500, "load settings: %v", err)
		return
	}
	if _, err := h.Store.UpsertAllocation(ctx, empID, typeID, year, days); err != nil {
		c.String(500, "save allocation: %v", err)
		return
	}
	toast(c, "Allocation saved.", "success")
	c.Status(200)
}
