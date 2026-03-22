package accounts

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the accounts Service and exposes it over HTTP.
type Handler struct {
	svc Service
}

// NewHandler constructs the accounts Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts all account endpoints onto the provided router group.
// Every route requires authentication — the caller is responsible for wrapping
// this group with middleware.Auth before calling RegisterRoutes.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("", h.create)
	rg.GET("", h.list)
	rg.GET("/:id", h.getByID)
	rg.GET("/:id/summary", h.getSummary)
	rg.PUT("/:id", h.update)
	rg.DELETE("/:id", h.delete)
}

// ── Handlers ───────────────────────────────────────────────────────────────────

// create godoc
//
//	@Summary      Create an account
//	@Description  Creates a new financial account (bank, cash, credit_card, investment) for the authenticated user.
//	@Tags         accounts
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      CreateAccountRequest    true  "Account details"
//	@Success      201   {object}  map[string]interface{}  "Account created"
//	@Failure      400   {object}  map[string]interface{}  "Validation error or invalid account type"
//	@Failure      401   {object}  map[string]interface{}  "Unauthorized"
//	@Failure      500   {object}  map[string]interface{}  "Internal server error"
//	@Router       /accounts [post]
func (h *Handler) create(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Always read userID from the verified JWT context — never trust the request body.
	userID := middleware.GetUserID(c)

	account, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidAccountType):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		default:
			slog.Error("accounts.create: unexpected error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create account"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": account})
}

// list godoc
//
//	@Summary      List accounts
//	@Description  Returns all financial accounts belonging to the authenticated user.
//	@Tags         accounts
//	@Produce      json
//	@Security     BearerAuth
//	@Success      200   {object}  map[string]interface{}  "List of accounts with total count"
//	@Failure      401   {object}  map[string]interface{}  "Unauthorized"
//	@Failure      500   {object}  map[string]interface{}  "Internal server error"
//	@Router       /accounts [get]
func (h *Handler) list(c *gin.Context) {
	userID := middleware.GetUserID(c)

	result, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		slog.Error("accounts.list: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list accounts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// getByID godoc
//
//	@Summary      Get account by ID
//	@Description  Returns a single account. Returns 404 if the account does not exist or belongs to another user.
//	@Tags         accounts
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                     true  "Account ID"
//	@Success      200  {object}  map[string]interface{}  "Account details"
//	@Failure      401  {object}  map[string]interface{}  "Unauthorized"
//	@Failure      404  {object}  map[string]interface{}  "Account not found"
//	@Failure      500  {object}  map[string]interface{}  "Internal server error"
//	@Router       /accounts/{id} [get]
func (h *Handler) getByID(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	account, err := h.svc.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
			return
		}
		slog.Error("accounts.getByID: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to fetch account"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": account})
}

// getSummary godoc
//
//	@Summary      Get account summary
//	@Description  Returns account details enriched with transaction count and last transaction date.
//	@Tags         accounts
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                     true  "Account ID"
//	@Success      200  {object}  map[string]interface{}  "Account summary"
//	@Failure      401  {object}  map[string]interface{}  "Unauthorized"
//	@Failure      404  {object}  map[string]interface{}  "Account not found"
//	@Failure      500  {object}  map[string]interface{}  "Internal server error"
//	@Router       /accounts/{id}/summary [get]
func (h *Handler) getSummary(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	summary, err := h.svc.GetSummary(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
			return
		}
		slog.Error("accounts.getSummary: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to fetch account summary"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

// update godoc
//
//	@Summary      Update account
//	@Description  Updates name, type, or active status of an account. Balance and currency cannot be changed via this endpoint.
//	@Tags         accounts
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id    path      int                     true  "Account ID"
//	@Param        body  body      UpdateAccountRequest    true  "Fields to update"
//	@Success      200   {object}  map[string]interface{}  "Updated account"
//	@Failure      400   {object}  map[string]interface{}  "Validation error"
//	@Failure      401   {object}  map[string]interface{}  "Unauthorized"
//	@Failure      404   {object}  map[string]interface{}  "Account not found"
//	@Failure      500   {object}  map[string]interface{}  "Internal server error"
//	@Router       /accounts/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)

	account, err := h.svc.Update(c.Request.Context(), id, userID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrAccountNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
		case errors.Is(err, ErrInvalidAccountType):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		case errors.Is(err, ErrNoUpdates):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		default:
			slog.Error("accounts.update: unexpected error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update account"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": account})
}

// delete godoc
//
//	@Summary      Delete account
//	@Description  Soft-deletes an account. Returns 409 Conflict if the account has existing transactions.
//	@Tags         accounts
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                     true  "Account ID"
//	@Success      200  {object}  map[string]interface{}  "Deleted successfully"
//	@Failure      401  {object}  map[string]interface{}  "Unauthorized"
//	@Failure      404  {object}  map[string]interface{}  "Account not found"
//	@Failure      409  {object}  map[string]interface{}  "Account has transactions"
//	@Failure      500  {object}  map[string]interface{}  "Internal server error"
//	@Router       /accounts/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	if err := h.svc.Delete(c.Request.Context(), id, userID); err != nil {
		switch {
		case errors.Is(err, ErrAccountNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
		case errors.Is(err, ErrHasTransactions):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
		default:
			slog.Error("accounts.delete: unexpected error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to delete account"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Account deleted successfully"}})
}

// ── Private helpers ────────────────────────────────────────────────────────────

// parseID extracts and validates a path parameter as a uint.
// Writes a 400 response and returns false when the parameter is missing or not a
// positive integer, so callers can return immediately on false.
func parseID(c *gin.Context, param string) (uint, bool) {
	raw := c.Param(param)
	val, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || val == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid " + param})
		return 0, false
	}
	return uint(val), true
}
