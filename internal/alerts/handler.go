package alerts

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the alerts Service and exposes it over HTTP via Gin.
type Handler struct {
	svc Service
}

// NewHandler constructs the alerts Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts all alert endpoints onto the provided router group.
// Every route requires authentication — the caller must apply middleware.Auth
// to the group before calling this function.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.list)
	rg.PATCH("/:id/read", h.markRead)
	rg.PATCH("/read-all", h.markAllRead)
	rg.DELETE("/:id", h.delete)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// list godoc
//
//	@Summary      List alerts
//	@Description  Returns all in-app alerts for the authenticated user (newest first). Includes total count and unread count in the response envelope.
//	@Tags         alerts
//	@Produce      json
//	@Security     BearerAuth
//	@Success      200  {object}  map[string]interface{} "Alerts with total and unread_count"
//	@Failure      401  {object}  map[string]interface{} "Unauthorized"
//	@Failure      500  {object}  map[string]interface{} "Internal server error"
//	@Router       /alerts [get]
func (h *Handler) list(c *gin.Context) {
	userID := middleware.GetUserID(c)

	result, err := h.svc.List(c.Request.Context(), userID)
	if err != nil {
		slog.Error("alerts.list: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list alerts"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// markRead godoc
//
//	@Summary      Mark alert as read
//	@Description  Sets is_read = true on a single alert. Returns 404 if the alert does not exist or belongs to another user.
//	@Tags         alerts
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                    true  "Alert ID"
//	@Success      200  {object}  map[string]interface{} "Alert marked as read"
//	@Failure      401  {object}  map[string]interface{} "Unauthorized"
//	@Failure      404  {object}  map[string]interface{} "Alert not found"
//	@Failure      500  {object}  map[string]interface{} "Internal server error"
//	@Router       /alerts/{id}/read [patch]
func (h *Handler) markRead(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	if err := h.svc.MarkRead(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, ErrAlertNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Alert not found"})
			return
		}
		slog.Error("alerts.markRead: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to mark alert as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Alert marked as read"}})
}

// markAllRead godoc
//
//	@Summary      Mark all alerts as read
//	@Description  Sets is_read = true on every unread alert for the authenticated user.
//	@Tags         alerts
//	@Produce      json
//	@Security     BearerAuth
//	@Success      200  {object}  map[string]interface{} "All alerts marked as read"
//	@Failure      401  {object}  map[string]interface{} "Unauthorized"
//	@Failure      500  {object}  map[string]interface{} "Internal server error"
//	@Router       /alerts/read-all [patch]
func (h *Handler) markAllRead(c *gin.Context) {
	userID := middleware.GetUserID(c)

	if err := h.svc.MarkAllRead(c.Request.Context(), userID); err != nil {
		slog.Error("alerts.markAllRead: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to mark alerts as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "All alerts marked as read"}})
}

// delete godoc
//
//	@Summary      Delete alert
//	@Description  Permanently deletes an alert. Unlike soft-deletes used elsewhere, alerts are hard-deleted to keep the table lean.
//	@Tags         alerts
//	@Produce      json
//	@Security     BearerAuth
//	@Param        id   path      int                    true  "Alert ID"
//	@Success      200  {object}  map[string]interface{} "Alert deleted"
//	@Failure      401  {object}  map[string]interface{} "Unauthorized"
//	@Failure      404  {object}  map[string]interface{} "Alert not found"
//	@Failure      500  {object}  map[string]interface{} "Internal server error"
//	@Router       /alerts/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	userID := middleware.GetUserID(c)

	if err := h.svc.Delete(c.Request.Context(), id, userID); err != nil {
		if errors.Is(err, ErrAlertNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Alert not found"})
			return
		}
		slog.Error("alerts.delete: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to delete alert"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Alert deleted"}})
}

// ── Helpers ────────────────────────────────────────────────────────────────────

// parseID extracts a uint path parameter from the Gin context.
// Writes a 400 response and returns false when the value is missing or invalid.
func parseID(c *gin.Context, param string) (uint, bool) {
	raw := c.Param(param)
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid " + param})
		return 0, false
	}
	return uint(v), true
}
