package budgets

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the budgets Service and exposes it over HTTP via Gin.
type Handler struct {
	svc Service
}

// NewHandler constructs the budgets Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts all budget endpoints onto the provided router group.
// Every route requires authentication — the caller must apply middleware.Auth
// to the group before calling this function.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.create)
	rg.GET("", h.list)
	rg.GET("/:id", h.getByID)
	rg.PUT("/:id", h.update)
	rg.DELETE("/:id", h.delete)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// create godoc
//
//	@Summary      Create a budget
//	@Description  Creates a new spending budget for a category and period. Spent/remaining values are computed live from transactions.
//	@Tags         budgets
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      CreateBudgetRequest    true  "Budget details"
//	@Success      201   {object}  map[string]interface{} "Budget created with live progress"
//	@Failure      400   {object}  map[string]interface{} "Validation error"
//	@Failure      401   {object}  map[string]interface{} "Unauthorized"
//	@Failure      500   {object}  map[string]interface{} "Internal server error"
//	@Router       /budgets [post]
func (h *Handler) create(c *gin.Context) {
	var req CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Always read userID from the verified JWT context — never from the request body.
	userID := middleware.GetUserID(c)

	resp, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		h.handleError(c, err, "create")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": resp})
}

// list godoc
//
//	@Summary      List budgets
//	@Description  Returns all budgets for the authenticated user. Each budget includes live spent, remaining, and percent_used fields computed from transactions.
//	@Tags         budgets
//	@Produce      json
//	@Security     BearerAuth
//	@Success      200  {object}  map[string]interface{} "List of budgets with progress"
//	@Failure      401  {object}  map[string]interface{} "Unauthorized"
//	@Failure      500  {object}  map[string]interface{} "Internal server error"
//	@Router       /budgets [get]
func (h *Handler) list(c *gin.Context) {
	userID := middleware.GetUserID(c)

	result, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		slog.Error("budgets.list: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list budgets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// getByID godoc
//
//	@Summary      Get budget by ID
//	@Description  Returns a single budget with live spent/remaining/percent_used values. Returns 404 if the budget does not exist or belongs to another user.
//	@Tags         budgets
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                    true  "Budget ID"
//	@Success      200  {object}  map[string]interface{} "Budget details with live progress"
//	@Failure      401  {object}  map[string]interface{} "Unauthorized"
//	@Failure      404  {object}  map[string]interface{} "Budget not found"
//	@Failure      500  {object}  map[string]interface{} "Internal server error"
//	@Router       /budgets/{id} [get]
func (h *Handler) getByID(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	resp, err := h.svc.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		h.handleError(c, err, "getByID")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// update godoc
//
//	@Summary      Update budget
//	@Description  Updates name, amount, period, end_date, or active status of a budget. All fields are optional.
//	@Tags         budgets
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id    path      int                    true  "Budget ID"
//	@Param        body  body      UpdateBudgetRequest    true  "Fields to update"
//	@Success      200   {object}  map[string]interface{} "Updated budget with live progress"
//	@Failure      400   {object}  map[string]interface{} "Validation error"
//	@Failure      401   {object}  map[string]interface{} "Unauthorized"
//	@Failure      404   {object}  map[string]interface{} "Budget not found"
//	@Failure      500   {object}  map[string]interface{} "Internal server error"
//	@Router       /budgets/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	var req UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)

	resp, err := h.svc.Update(c.Request.Context(), id, userID, req)
	if err != nil {
		h.handleError(c, err, "update")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// delete godoc
//
//	@Summary      Delete budget
//	@Description  Soft-deletes a budget. The record is retained in the database (deleted_at is set) but excluded from all list queries.
//	@Tags         budgets
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                    true  "Budget ID"
//	@Success      200  {object}  map[string]interface{} "Deletion confirmed"
//	@Failure      401  {object}  map[string]interface{} "Unauthorized"
//	@Failure      404  {object}  map[string]interface{} "Budget not found"
//	@Failure      500  {object}  map[string]interface{} "Internal server error"
//	@Router       /budgets/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	if err := h.svc.Delete(c.Request.Context(), id, userID); err != nil {
		h.handleError(c, err, "delete")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Budget deleted"}})
}

// ── Helpers ────────────────────────────────────────────────────────────────────

// handleError maps domain errors to HTTP status codes.
// It follows the same pattern used by the accounts and transactions handlers.
func (h *Handler) handleError(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, ErrBudgetNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Budget not found"})
	case errors.Is(err, ErrInvalidPeriod),
		errors.Is(err, ErrInvalidDate),
		errors.Is(err, ErrEndBeforeStart),
		errors.Is(err, ErrNoUpdates):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
	default:
		slog.Error("budgets."+op+": unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "An unexpected error occurred"})
	}
}

// parseID extracts a uint path parameter from the Gin context.
// Writes a 400 response and returns false if the parameter is missing or not
// a positive integer.
func parseID(c *gin.Context, param string) (uint, bool) {
	raw := c.Param(param)
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid " + param})
		return 0, false
	}
	return uint(v), true
}
