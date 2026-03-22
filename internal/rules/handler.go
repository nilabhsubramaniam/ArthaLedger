package rules

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the HTTP handlers for the rules resource.
type Handler struct {
	svc Service
}

// NewHandler creates a Handler backed by the given Service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts rule endpoints on the provided RouterGroup.
// All routes require a valid JWT (Auth middleware must be applied by the caller).
//
//	GET    /rules        → List
//	POST   /rules        → Create
//	DELETE /rules/:id    → Delete
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.POST("", h.create)
	rg.DELETE("/:id", h.delete)
}

// handleServiceError maps service-layer sentinel errors to HTTP status codes.
func handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrRuleNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
	case errors.Is(err, ErrDuplicateKeyword):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "internal server error"})
	}
}

// parseID extracts and validates the ":id" path parameter.
func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

// list godoc
//
//	@Summary      List categorization rules
//	@Description  Returns all keyword → category rules for the authenticated user,
//	              ordered by priority descending. The first matching rule wins on
//	              transaction create.
//	@Tags         rules
//	@Produce      json
//	@Security     BearerAuth
//	@Success      200  {object}  RuleListResponse
//	@Failure      401  {object}  map[string]interface{}
//	@Router       /rules [get]
func (h *Handler) list(c *gin.Context) {
	userID := middleware.GetUserID(c)
	resp, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// create godoc
//
//	@Summary      Create a categorization rule
//	@Description  Creates a keyword → category mapping. When a new transaction is
//	              saved without an explicit category_id, the description is matched
//	              against these rules — the highest-priority match wins.
//	              Keywords are case-insensitive. Duplicate keywords return 409.
//	@Tags         rules
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      CreateRuleRequest  true  "Rule details"
//	@Success      201   {object}  RuleResponse
//	@Failure      400   {object}  map[string]interface{}
//	@Failure      401   {object}  map[string]interface{}
//	@Failure      409   {object}  map[string]interface{}
//	@Router       /rules [post]
func (h *Handler) create(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	resp, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": resp})
}

// delete godoc
//
//	@Summary      Delete a categorization rule
//	@Description  Permanently deletes a rule. Existing transactions are not
//	              affected — only future transactions will stop being auto-categorized
//	              by this keyword.
//	@Tags         rules
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int  true  "Rule ID"
//	@Success      200  {object}  map[string]interface{}
//	@Failure      400  {object}  map[string]interface{}
//	@Failure      401  {object}  map[string]interface{}
//	@Failure      404  {object}  map[string]interface{}
//	@Router       /rules/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	userID := middleware.GetUserID(c)

	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.svc.Delete(c.Request.Context(), userID, id); err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "rule deleted"})
}
