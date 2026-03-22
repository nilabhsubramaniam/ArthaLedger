package transactions

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/internal/accounts"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the transactions Service and exposes it over HTTP via Gin.
type Handler struct {
	svc Service
}

// NewHandler constructs a transactions Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts all transaction endpoints.
// Every route requires authentication — the caller must wrap this group with
// middleware.Auth before calling RegisterRoutes.
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
//	@Summary      Create a transaction
//	@Description  Creates an income, expense, or transfer transaction. For transfers, provide to_account_id. Balance is updated atomically.
//	@Tags         transactions
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      CreateTransactionRequest  true  "Transaction payload"
//	@Success      201   {object}  map[string]interface{}    "Transaction created"
//	@Failure      400   {object}  map[string]interface{}    "Validation error"
//	@Failure      401   {object}  map[string]interface{}    "Unauthorized"
//	@Failure      404   {object}  map[string]interface{}    "Account not found"
//	@Failure      500   {object}  map[string]interface{}    "Internal server error"
//	@Router       /transactions [post]
func (h *Handler) create(c *gin.Context) {
	var req CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)

	txn, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		h.handleServiceError(c, err, "create")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": txn})
}

// list godoc
//
//	@Summary      List transactions
//	@Description  Returns a paginated, filtered list of transactions for the authenticated user.
//	@Tags         transactions
//	@Produce      json
//	@Security     BearerAuth
//	@Param        account_id   query     int     false  "Filter by account ID"
//	@Param        category_id  query     int     false  "Filter by category ID"
//	@Param        type         query     string  false  "Filter by type: income | expense | transfer"
//	@Param        date_from    query     string  false  "Start date (YYYY-MM-DD)"
//	@Param        date_to      query     string  false  "End date (YYYY-MM-DD)"
//	@Param        min_amount   query     number  false  "Minimum amount"
//	@Param        max_amount   query     number  false  "Maximum amount"
//	@Param        page         query     int     false  "Page number (default 1)"
//	@Param        limit        query     int     false  "Results per page (default 20, max 100)"
//	@Success      200          {object}  map[string]interface{}  "Paginated transaction list"
//	@Failure      401          {object}  map[string]interface{}  "Unauthorized"
//	@Failure      500          {object}  map[string]interface{}  "Internal server error"
//	@Router       /transactions [get]
func (h *Handler) list(c *gin.Context) {
	userID := middleware.GetUserID(c)

	// Parse all optional query parameters into the filter struct.
	f := TransactionFilter{
		Page:  parseIntQuery(c, "page", 1),
		Limit: parseIntQuery(c, "limit", 20),
	}

	if v := parseUintQuery(c, "account_id"); v != nil {
		f.AccountID = v
	}
	if v := parseUintQuery(c, "category_id"); v != nil {
		f.CategoryID = v
	}
	if t := c.Query("type"); t != "" {
		f.Type = TransactionType(t)
	}
	if d := parseDateQuery(c, "date_from"); d != nil {
		f.DateFrom = d
	}
	if d := parseDateQuery(c, "date_to"); d != nil {
		f.DateTo = d
	}
	if v := parseFloatQuery(c, "min_amount"); v != nil {
		f.MinAmount = v
	}
	if v := parseFloatQuery(c, "max_amount"); v != nil {
		f.MaxAmount = v
	}

	result, err := h.svc.List(c.Request.Context(), userID, f)
	if err != nil {
		slog.Error("transactions.list: error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list transactions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// getByID godoc
//
//	@Summary      Get transaction by ID
//	@Description  Returns a single transaction. Returns 404 if not found or owned by another user.
//	@Tags         transactions
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                     true  "Transaction ID"
//	@Success      200  {object}  map[string]interface{}  "Transaction details"
//	@Failure      401  {object}  map[string]interface{}  "Unauthorized"
//	@Failure      404  {object}  map[string]interface{}  "Transaction not found"
//	@Failure      500  {object}  map[string]interface{}  "Internal server error"
//	@Router       /transactions/{id} [get]
func (h *Handler) getByID(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	txn, err := h.svc.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		h.handleServiceError(c, err, "getByID")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": txn})
}

// update godoc
//
//	@Summary      Update a transaction
//	@Description  Updates amount, category, description, note, or date of a non-transfer transaction. Balance is corrected atomically.
//	@Tags         transactions
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id    path      int                       true  "Transaction ID"
//	@Param        body  body      UpdateTransactionRequest  true  "Fields to update"
//	@Success      200   {object}  map[string]interface{}    "Updated transaction"
//	@Failure      400   {object}  map[string]interface{}    "Validation error or transfer update attempted"
//	@Failure      401   {object}  map[string]interface{}    "Unauthorized"
//	@Failure      404   {object}  map[string]interface{}    "Transaction not found"
//	@Failure      500   {object}  map[string]interface{}    "Internal server error"
//	@Router       /transactions/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}

	var req UpdateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)

	txn, err := h.svc.Update(c.Request.Context(), id, userID, req)
	if err != nil {
		h.handleServiceError(c, err, "update")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": txn})
}

// delete godoc
//
//	@Summary      Delete a transaction
//	@Description  Soft-deletes a transaction and reverses its balance impact. For transfers, both legs are deleted atomically.
//	@Tags         transactions
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                     true  "Transaction ID"
//	@Success      200  {object}  map[string]interface{}  "Deleted successfully"
//	@Failure      401  {object}  map[string]interface{}  "Unauthorized"
//	@Failure      404  {object}  map[string]interface{}  "Transaction not found"
//	@Failure      500  {object}  map[string]interface{}  "Internal server error"
//	@Router       /transactions/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := parsePathID(c)
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	if err := h.svc.Delete(c.Request.Context(), id, userID); err != nil {
		h.handleServiceError(c, err, "delete")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Transaction deleted successfully"}})
}

// ── Shared error handler ──────────────────────────────────────────────────────

// handleServiceError maps domain errors to the correct HTTP status.
// Having one central mapping keeps all handlers consistent.
func (h *Handler) handleServiceError(c *gin.Context, err error, op string) {
	switch {
	case errors.Is(err, ErrTransactionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Transaction not found"})
	case errors.Is(err, accounts.ErrAccountNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
	case errors.Is(err, ErrInvalidTransactionType),
		errors.Is(err, ErrTransferRequiresToAccount),
		errors.Is(err, ErrSameAccountTransfer),
		errors.Is(err, ErrInvalidDate),
		errors.Is(err, ErrTransferNotEditable),
		errors.Is(err, ErrNoUpdates):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
	default:
		slog.Error("transactions."+op+": unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Internal server error"})
	}
}

// ── Query parameter helpers ───────────────────────────────────────────────────

// parsePathID extracts the ":id" path parameter and validates it as a positive uint.
// Writes a 400 response and returns false on failure.
func parsePathID(c *gin.Context) (uint, bool) {
	raw := c.Param("id")
	val, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || val == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid id"})
		return 0, false
	}
	return uint(val), true
}

// parseIntQuery parses a query parameter as an integer, falling back to defaultVal.
func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	raw := c.Query(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return defaultVal
	}
	return v
}

// parseUintQuery parses an optional query parameter as a *uint.
// Returns nil when the parameter is absent or invalid.
func parseUintQuery(c *gin.Context, key string) *uint {
	raw := c.Query(key)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		return nil
	}
	u := uint(v)
	return &u
}

// parseFloatQuery parses an optional query parameter as a *float64.
// Returns nil when absent or invalid.
func parseFloatQuery(c *gin.Context, key string) *float64 {
	raw := c.Query(key)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}

// parseDateQuery parses an optional "YYYY-MM-DD" query parameter as a *time.Time.
// Returns nil when absent or incorrectly formatted.
func parseDateQuery(c *gin.Context, key string) *time.Time {
	raw := c.Query(key)
	if raw == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil
	}
	return &t
}
