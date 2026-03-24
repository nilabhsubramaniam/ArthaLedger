package reports

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the reports Service and exposes it over HTTP via Gin.
type Handler struct {
	svc Service
}

// NewHandler constructs the reports Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts all report endpoints onto the provided router group.
// Every route requires authentication — the caller must apply middleware.Auth
// to the group before calling this function.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/monthly", h.monthly)
	rg.GET("/trends", h.trends)
	rg.GET("/export", h.export)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// monthly godoc
//
//	@Summary      Monthly financial summary
//	@Description  Returns total income, total expenses, net savings, and a per-category expense breakdown for the given calendar month.
//	@Tags         reports
//	@Produce      json
//	@Security     BearerAuth
//	@Param        year   query     int  true   "Calendar year  (e.g. 2026)"
//	@Param        month  query     int  true   "Calendar month (1–12)"
//	@Success      200    {object}  map[string]interface{} "Monthly summary"
//	@Failure      400    {object}  map[string]interface{} "Invalid year/month or future date"
//	@Failure      401    {object}  map[string]interface{} "Unauthorized"
//	@Failure      500    {object}  map[string]interface{} "Internal server error"
//	@Router       /reports/monthly [get]
func (h *Handler) monthly(c *gin.Context) {
	year, month, ok := h.parseYearMonth(c)
	if !ok {
		return
	}

	userID := middleware.GetUserID(c)

	summary, err := h.svc.GetMonthlySummary(c.Request.Context(), userID, year, month)
	if err != nil {
		h.handleError(c, err, "monthly")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// trends godoc
//
//	@Summary      Spending trends
//	@Description  Returns income, expense, and net figures for each of the last N calendar months. Defaults to 6 months; max 24.
//	@Tags         reports
//	@Produce      json
//	@Security     BearerAuth
//	@Param        months  query     int  false  "Number of months to include (default 6, max 24)"
//	@Success      200     {object}  map[string]interface{} "Trend data"
//	@Failure      400     {object}  map[string]interface{} "Invalid months value"
//	@Failure      401     {object}  map[string]interface{} "Unauthorized"
//	@Failure      500     {object}  map[string]interface{} "Internal server error"
//	@Router       /reports/trends [get]
func (h *Handler) trends(c *gin.Context) {
	months := 6 // default
	if raw := c.Query("months"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 24 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": ErrInvalidMonths.Error()})
			return
		}
		months = v
	}

	userID := middleware.GetUserID(c)

	result, err := h.svc.GetTrend(c.Request.Context(), userID, months)
	if err != nil {
		h.handleError(c, err, "trends")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// export godoc
//
//	@Summary      Export transactions as CSV
//	@Description  Streams all transactions for the given month as a CSV file download. Columns: Date, Description, Amount, Type, Category, Account, Note.
//	@Tags         reports
//	@Produce      text/csv
//	@Security     BearerAuth
//	@Param        year   query     int  true   "Calendar year  (e.g. 2026)"
//	@Param        month  query     int  true   "Calendar month (1–12)"
//	@Success      200    {file}    string "CSV file download"
//	@Failure      400    {object}  map[string]interface{} "Invalid year/month"
//	@Failure      401    {object}  map[string]interface{} "Unauthorized"
//	@Failure      500    {object}  map[string]interface{} "Internal server error"
//	@Router       /reports/export [get]
func (h *Handler) export(c *gin.Context) {
	year, month, ok := h.parseYearMonth(c)
	if !ok {
		return
	}

	userID := middleware.GetUserID(c)

	rows, err := h.svc.GetExportRows(c.Request.Context(), userID, year, month)
	if err != nil {
		h.handleError(c, err, "export")
		return
	}

	// Set response headers for file download.
	filename := fmt.Sprintf("transactions-%d-%02d.csv", year, month)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Status(http.StatusOK)

	// Stream rows directly to the response writer — no intermediate buffer.
	w := csv.NewWriter(c.Writer)

	// Header row
	_ = w.Write([]string{"Date", "Description", "Amount", "Type", "Category", "Account", "Note"})

	for _, row := range rows {
		_ = w.Write([]string{
			row.Date.Format("2006-01-02"),
			row.Description,
			strconv.FormatFloat(row.Amount, 'f', 2, 64),
			row.Type,
			row.Category,
			row.AccountName,
			row.Note,
		})
	}

	w.Flush()

	if err := w.Error(); err != nil {
		// At this point response headers are already sent; just log the error.
		slog.Error("reports.export: csv flush error", "error", err)
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────────

// parseYearMonth extracts and validates the "year" and "month" query parameters.
// Writes a 400 response and returns false when either is missing or invalid.
func (h *Handler) parseYearMonth(c *gin.Context) (int, int, bool) {
	rawYear := c.Query("year")
	rawMonth := c.Query("month")

	if rawYear == "" || rawMonth == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "year and month query parameters are required"})
		return 0, 0, false
	}

	year, err := strconv.Atoi(rawYear)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": ErrInvalidYear.Error()})
		return 0, 0, false
	}

	month, err := strconv.Atoi(rawMonth)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": ErrInvalidMonth.Error()})
		return 0, 0, false
	}

	return year, month, true
}

// handleError maps domain errors to HTTP status codes.
func (h *Handler) handleError(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, ErrInvalidYear),
		errors.Is(err, ErrInvalidMonth),
		errors.Is(err, ErrInvalidMonths),
		errors.Is(err, ErrFutureMonth):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
	default:
		slog.Error("reports."+op+": unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to generate report"})
	}
}

// currentYearMonth returns the current year and month — used as the default
// when year/month parameters are not supplied to a handler that needs a default.
func currentYearMonth() (int, int) { //nolint:unused // reserved for future handler defaults
	now := time.Now()
	return now.Year(), int(now.Month())
}
