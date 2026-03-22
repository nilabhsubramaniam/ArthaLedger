package categories

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the HTTP handlers for the categories resource.
type Handler struct {
	svc Service
}

// NewHandler creates a Handler with the given Service.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts category endpoints on the provided RouterGroup.
// All routes require a valid JWT (the caller is responsible for applying
// the Auth middleware before passing the group here).
//
//	GET    /categories           → List
//	POST   /categories           → Create
//	GET    /categories/:id       → GetByID
//	PUT    /categories/:id       → Update
//	DELETE /categories/:id       → Delete
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.POST("", h.create)
	rg.GET("/:id", h.getByID)
	rg.PUT("/:id", h.update)
	rg.DELETE("/:id", h.delete)
}

// handleServiceError maps service-layer sentinel errors to the correct HTTP
// status code and writes a JSON error response.
func handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrCategoryNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
	case errors.Is(err, ErrSystemCategory):
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": err.Error()})
	case errors.Is(err, ErrInvalidCategoryType):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
	case errors.Is(err, ErrNoUpdates):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "internal server error"})
	}
}

// parseID extracts and validates the ":id" path parameter.
func parseID(c *gin.Context) (uint, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

// list godoc
//
//	@Summary      List categories
//	@Description  Returns all system categories plus the authenticated user's own categories.
//	              Optionally filter by type (income or expense).
//	@Tags         categories
//	@Produce      json
//	@Security     BearerAuth
//	@Param        type  query     string  false  "Filter by type: income or expense"
//	@Success      200   {object}  CategoryListResponse
//	@Failure      400   {object}  map[string]interface{}
//	@Failure      401   {object}  map[string]interface{}
//	@Router       /categories [get]
func (h *Handler) list(c *gin.Context) {
	userID := middleware.GetUserID(c)
	categoryType := c.Query("type") // optional filter

	resp, err := h.svc.List(c.Request.Context(), userID, categoryType)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// create godoc
//
//	@Summary      Create a category
//	@Description  Creates a new user-defined category (income or expense).
//	              System categories cannot be created via this endpoint.
//	@Tags         categories
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      CreateCategoryRequest  true  "Category details"
//	@Success      201   {object}  CategoryResponse
//	@Failure      400   {object}  map[string]interface{}
//	@Failure      401   {object}  map[string]interface{}
//	@Router       /categories [post]
func (h *Handler) create(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var req CreateCategoryRequest
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

// getByID godoc
//
//	@Summary      Get category by ID
//	@Description  Returns a single category. The category must be a system category
//	              or belong to the authenticated user.
//	@Tags         categories
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int  true  "Category ID"
//	@Success      200  {object}  CategoryResponse
//	@Failure      400  {object}  map[string]interface{}
//	@Failure      401  {object}  map[string]interface{}
//	@Failure      404  {object}  map[string]interface{}
//	@Router       /categories/{id} [get]
func (h *Handler) getByID(c *gin.Context) {
	userID := middleware.GetUserID(c)

	id, ok := parseID(c)
	if !ok {
		return
	}

	resp, err := h.svc.GetByID(c.Request.Context(), userID, id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// update godoc
//
//	@Summary      Update a category
//	@Description  Updates the name, icon, or color of a user-owned category.
//	              System categories (is_system=true) cannot be updated.
//	@Tags         categories
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id    path      int                    true  "Category ID"
//	@Param        body  body      UpdateCategoryRequest  true  "Fields to update"
//	@Success      200   {object}  CategoryResponse
//	@Failure      400   {object}  map[string]interface{}
//	@Failure      401   {object}  map[string]interface{}
//	@Failure      403   {object}  map[string]interface{}
//	@Failure      404   {object}  map[string]interface{}
//	@Router       /categories/{id} [put]
func (h *Handler) update(c *gin.Context) {
	userID := middleware.GetUserID(c)

	id, ok := parseID(c)
	if !ok {
		return
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	resp, err := h.svc.Update(c.Request.Context(), userID, id, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// delete godoc
//
//	@Summary      Delete a category
//	@Description  Permanently deletes a user-owned category.
//	              System categories (is_system=true) cannot be deleted.
//	              Transactions referencing this category will have category_id set to NULL.
//	@Tags         categories
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int  true  "Category ID"
//	@Success      200  {object}  map[string]interface{}
//	@Failure      400  {object}  map[string]interface{}
//	@Failure      401  {object}  map[string]interface{}
//	@Failure      403  {object}  map[string]interface{}
//	@Failure      404  {object}  map[string]interface{}
//	@Router       /categories/{id} [delete]
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
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "category deleted"})
}
