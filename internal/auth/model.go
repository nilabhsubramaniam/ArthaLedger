// Package auth implements user registration, login, JWT token issuance,
// token refresh, and logout (token blacklisting).
//
// Layer responsibilities in this package:
//
//	model.go      — GORM structs and HTTP request/response types
//	repository.go — database queries (interface + PostgreSQL impl)
//	service.go    — business logic (interface + concrete impl)
//	handler.go    — HTTP handlers wired to Gin routes
package auth

import (
	"time"

	"gorm.io/gorm"
)

// ── Database model ─────────────────────────────────────────────────────────────

// User is the GORM model that maps to the `users` table created in migration 000001.
// The Password field is tagged `json:"-"` so it is NEVER included in any JSON
// response, regardless of how the struct is serialized.
type User struct {
	ID        uint           `gorm:"primaryKey"                         json:"id"`
	Name      string         `gorm:"type:varchar(255);not null"         json:"name"`
	Email     string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"email"`
	Password  string         `gorm:"type:varchar(255);not null"         json:"-"` // bcrypt hash — never serialized
	IsActive  bool           `gorm:"default:true"                       json:"is_active"`
	CreatedAt time.Time      `                                          json:"created_at"`
	UpdatedAt time.Time      `                                          json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                              json:"-"` // soft-delete support
}

// ── HTTP request types ─────────────────────────────────────────────────────────

// RegisterRequest is the JSON payload expected by POST /auth/register.
// Field validation is enforced by Gin's binding engine via struct tags.
type RegisterRequest struct {
	Name     string `json:"name"     binding:"required,min=1,max=255"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"` // plain-text; bcrypt-hashed in service
}

// LoginRequest is the JSON payload expected by POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest carries the opaque refresh token UUID issued at login.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// LogoutRequest optionally carries the refresh token so it can be revoked
// alongside the access token. Omitting it only blacklists the access token.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// ── HTTP response types ────────────────────────────────────────────────────────

// UserResponse is the safe public view of a User — no password field.
// Returned by the register endpoint.
type UserResponse struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// TokenPair is returned by the login endpoint.
// AccessToken is a short-lived JWT; RefreshToken is a long-lived opaque UUID.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds until the access token expires
}

// AccessTokenResponse is returned by the refresh endpoint.
// Only a new access token is issued — the refresh token is not rotated.
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// toUserResponse converts a User model to its safe public representation.
// Centralised here so handlers never accidentally return the full struct.
func toUserResponse(u *User) *UserResponse {
	return &UserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt,
	}
}
