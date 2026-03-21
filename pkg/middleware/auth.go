// Package middleware provides Gin HTTP middleware used across all routes.
// Currently contains:
//   - Auth: JWT Bearer token validation + Redis blacklist check
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/nilabh/arthaledger/config"
	"github.com/redis/go-redis/v9"
)

// ── Context keys ───────────────────────────────────────────────────────────────
// Typed string constants prevent accidental key collisions when multiple
// middleware functions write to the same Gin context.

const (
	ContextKeyUserID = "user_id"      // uint  — authenticated user's primary key
	ContextKeyEmail  = "email"        // string — authenticated user's email address
	ContextKeyJTI    = "jti"          // string — JWT ID, used for blacklisting on logout
	ContextKeyExpiry = "token_expiry" // time.Time — when the access token expires
)

// ── Claims ─────────────────────────────────────────────────────────────────────

// Claims mirrors the JWT payload defined in the auth package.
// It is intentionally redefined here rather than imported from `internal/auth`
// to avoid a circular dependency (middleware → auth → middleware would be a cycle).
type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// ── Middleware ─────────────────────────────────────────────────────────────────

// Auth returns a Gin middleware that enforces JWT authentication on every request.
//
// Steps performed on each request:
//  1. Extract the Bearer token from the Authorization header.
//  2. Parse and validate the JWT signature (HS256 only) and expiry.
//  3. Check the token's JTI against the Redis blacklist (populated on logout).
//  4. Store user_id, email, jti, and token expiry in the Gin context so that
//     downstream handlers can call GetUserID(), GetJTI(), etc. without re-parsing.
//
// redisClient may be nil (development mode without Redis running).
// When nil, the blacklist check is skipped — tokens cannot be proactively revoked
// until they expire naturally, which is acceptable in local development.
func Auth(cfg *config.Config, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		// ── Step 1: Parse the Authorization header ────────────────────────────
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header is required",
			})
			return
		}

		// Expected format: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization header format must be: Bearer <token>",
			})
			return
		}
		tokenStr := parts[1]

		// ── Step 2: Parse & validate the JWT ─────────────────────────────────
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(
			tokenStr,
			claims,
			func(t *jwt.Token) (interface{}, error) {
				// Reject any algorithm other than HS256.
				// Without this check a crafted "alg: none" header bypasses verification.
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(cfg.JWT.Secret), nil
			},
		)
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or expired token",
			})
			return
		}

		// ── Step 3: Redis blacklist check ─────────────────────────────────────
		// The logout handler writes "blacklist:<jti>" = "1" with a TTL equal to
		// the token's remaining lifetime. If the key exists, the token was revoked.
		if redisClient != nil {
			blacklistKey := fmt.Sprintf("blacklist:%s", claims.ID)
			exists, err := redisClient.Exists(context.Background(), blacklistKey).Result()
			if err == nil && exists > 0 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   "Token has been revoked",
				})
				return
			}
			// A Redis error here is non-fatal — we allow the request through rather
			// than taking the entire service down because Redis is temporarily unavailable.
		}

		// ── Step 4: Stash claims in context ───────────────────────────────────
		// Handlers retrieve these with the typed helpers below (GetUserID, etc.)
		// rather than calling c.Get() directly, so the type assertions are centralised.
		c.Set(ContextKeyUserID, claims.UserID)
		c.Set(ContextKeyEmail, claims.Email)
		c.Set(ContextKeyJTI, claims.ID)             // ID = JTI in RegisteredClaims
		c.Set(ContextKeyExpiry, claims.ExpiresAt.Time)

		c.Next()
	}
}

// ── Context helpers ────────────────────────────────────────────────────────────
// These helpers centralise type assertions so handlers never call c.Get() directly.
// They return zero values when the auth middleware was not applied to the route.

// GetUserID returns the authenticated user's primary key from the Gin context.
// Returns 0 if the auth middleware was not applied.
func GetUserID(c *gin.Context) uint {
	val, _ := c.Get(ContextKeyUserID)
	id, _ := val.(uint)
	return id
}

// GetEmail returns the authenticated user's email address from the Gin context.
func GetEmail(c *gin.Context) string {
	val, _ := c.Get(ContextKeyEmail)
	email, _ := val.(string)
	return email
}

// GetJTI returns the JWT ID claim from the Gin context.
// Used by the logout handler to blacklist the current token in Redis.
func GetJTI(c *gin.Context) string {
	val, _ := c.Get(ContextKeyJTI)
	jti, _ := val.(string)
	return jti
}

// GetTokenExpiry returns the token's expiry time from the Gin context.
// Used by the logout handler to calculate the blacklist TTL so that Redis
// does not retain the key after the token would have expired naturally.
func GetTokenExpiry(c *gin.Context) time.Time {
	val, _ := c.Get(ContextKeyExpiry)
	t, _ := val.(time.Time)
	return t
}
