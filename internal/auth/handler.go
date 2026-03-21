package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nilabh/arthaledger/pkg/middleware"
)

// Handler holds the auth Service and exposes it over HTTP via Gin.
type Handler struct {
	svc Service
}

// NewHandler creates a new auth Handler with the given service implementation.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the PUBLIC auth endpoints onto the supplied router group.
// These routes do not require a Bearer token:
//   - POST /register  — create an account
//   - POST /login     — exchange credentials for tokens
//   - POST /refresh   — exchange a refresh token for a new access token
//
// The logout endpoint is intentionally omitted here because it requires the
// auth middleware. Wire it via LogoutHandler() in the protected route group
// (see cmd/server/main.go).
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/register", h.register)
	rg.POST("/login", h.login)
	rg.POST("/refresh", h.refresh)
}

// LogoutHandler returns the gin.HandlerFunc for DELETE /auth/logout.
// Mount this on a route group that is already wrapped with middleware.Auth
// so that the JTI and expiry are present in the Gin context.
func (h *Handler) LogoutHandler() gin.HandlerFunc {
	return h.logout
}

// ── Handlers ───────────────────────────────────────────────────────────────────

// register godoc
//
//	@Summary      Register a new user
//	@Description  Creates a new user account. The password is bcrypt-hashed (cost 12) before storage and is never returned in any response.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      RegisterRequest         true  "Registration payload"
//	@Success      201   {object}  map[string]interface{}  "User created successfully"
//	@Failure      400   {object}  map[string]interface{}  "Validation error — missing or malformed fields"
//	@Failure      409   {object}  map[string]interface{}  "Email is already registered"
//	@Failure      500   {object}  map[string]interface{}  "Internal server error"
//	@Router       /auth/register [post]
func (h *Handler) register(c *gin.Context) {
	var req RegisterRequest
	// ShouldBindJSON validates all `binding:"..."` tags defined on RegisterRequest.
	// It returns a descriptive error that we surface directly to the caller.
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailTaken):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "Email already registered"})
		default:
			slog.Error("register: unexpected error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Registration failed"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": user})
}

// login godoc
//
//	@Summary      Login
//	@Description  Authenticates the user with email + password and returns a short-lived JWT access token and a long-lived opaque refresh token.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      LoginRequest            true  "Login credentials"
//	@Success      200   {object}  map[string]interface{}  "Token pair (access_token + refresh_token)"
//	@Failure      400   {object}  map[string]interface{}  "Validation error"
//	@Failure      401   {object}  map[string]interface{}  "Invalid email or password"
//	@Failure      403   {object}  map[string]interface{}  "Account deactivated"
//	@Failure      500   {object}  map[string]interface{}  "Internal server error"
//	@Router       /auth/login [post]
func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	tokens, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCreds):
			// 401 — deliberately vague to prevent email enumeration.
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid email or password"})
		case errors.Is(err, ErrInactiveUser):
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Account is deactivated"})
		default:
			slog.Error("login: unexpected error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Login failed"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tokens})
}

// refresh godoc
//
//	@Summary      Refresh access token
//	@Description  Exchanges a valid refresh token for a new JWT access token. The refresh token is NOT rotated on each use.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Param        body  body      RefreshRequest          true  "Refresh token payload"
//	@Success      200   {object}  map[string]interface{}  "New access token"
//	@Failure      400   {object}  map[string]interface{}  "Validation error"
//	@Failure      401   {object}  map[string]interface{}  "Invalid or expired refresh token"
//	@Router       /auth/refresh [post]
func (h *Handler) refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	token, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": token})
}

// logout godoc
//
//	@Summary      Logout
//	@Description  Revokes the current access token (JTI blacklisted in Redis) and optionally revokes the refresh token. Requires a valid Bearer token.
//	@Tags         auth
//	@Accept       json
//	@Produce      json
//	@Security     BearerAuth
//	@Param        body  body      LogoutRequest           false  "Optionally include refresh_token to revoke it"
//	@Success      200   {object}  map[string]interface{}  "Logged out successfully"
//	@Failure      401   {object}  map[string]interface{}  "Unauthorized — missing or invalid Bearer token"
//	@Failure      500   {object}  map[string]interface{}  "Internal server error"
//	@Router       /auth/logout [delete]
func (h *Handler) logout(c *gin.Context) {
	var req LogoutRequest
	// refresh_token is optional — ignore the bind error if the body is empty/missing.
	_ = c.ShouldBindJSON(&req)

	// The auth middleware (middleware.Auth) already validated the Bearer token and
	// stashed the JTI and expiry into the Gin context. We retrieve them here so
	// the service can calculate the correct blacklist TTL.
	jti := middleware.GetJTI(c)
	expiry := middleware.GetTokenExpiry(c)

	if err := h.svc.Logout(c.Request.Context(), jti, expiry, req.RefreshToken); err != nil {
		slog.Error("logout: unexpected error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Logout failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Logged out successfully"}})
}
