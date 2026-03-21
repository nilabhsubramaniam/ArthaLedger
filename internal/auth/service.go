package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/nilabh/arthaledger/config"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ── Sentinel errors ────────────────────────────────────────────────────────────
// Handlers check for these with errors.Is() to decide the correct HTTP status.

var (
	// ErrEmailTaken is returned by Register when the email is already in use.
	ErrEmailTaken = errors.New("email already registered")

	// ErrInvalidCreds is returned by Login for any credential failure.
	// A single generic message prevents email-enumeration attacks.
	ErrInvalidCreds = errors.New("invalid email or password")

	// ErrInactiveUser is returned when a valid user's account has been deactivated.
	ErrInactiveUser = errors.New("account is deactivated")

	// ErrInvalidToken is returned by Refresh when the token is missing, expired, or revoked.
	ErrInvalidToken = errors.New("invalid or expired token")
)

// ── JWT Claims ─────────────────────────────────────────────────────────────────

// Claims are the custom fields embedded inside every JWT access token.
// jwt.RegisteredClaims provides standard fields (sub, iat, exp, jti).
type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// ── Service interface ──────────────────────────────────────────────────────────

// Service is the business-logic contract for auth operations.
// Declaring an interface here lets future code (e.g. tests) inject a mock.
type Service interface {
	Register(ctx context.Context, req RegisterRequest) (*UserResponse, error)
	Login(ctx context.Context, req LoginRequest) (*TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*AccessTokenResponse, error)
	// Logout invalidates the access token JTI and revokes the refresh token.
	// jti and expiry come from the parsed JWT (set in context by the auth middleware).
	Logout(ctx context.Context, jti string, expiry time.Time, refreshToken string) error
}

// ── Concrete implementation ────────────────────────────────────────────────────

// service is the production implementation of Service.
type service struct {
	repo  Repository
	redis *redis.Client // may be nil in development (no Redis running)
	cfg   *config.Config
}

// NewService constructs the auth service.
// redisClient may be nil — all Redis operations gracefully degrade when it is.
func NewService(repo Repository, redisClient *redis.Client, cfg *config.Config) Service {
	return &service{
		repo:  repo,
		redis: redisClient,
		cfg:   cfg,
	}
}

// ── Service method implementations ────────────────────────────────────────────

// Register creates a new user account.
//
// Flow:
//  1. Verify the email is not already taken (DB lookup).
//  2. Hash the password with bcrypt (cost 12 — OWASP recommended minimum).
//  3. Insert the user row.
//  4. Return the safe public response (no password field).
func (s *service) Register(ctx context.Context, req RegisterRequest) (*UserResponse, error) {
	// Check uniqueness before hashing — avoids expensive bcrypt work on duplicates.
	_, err := s.repo.FindByEmail(ctx, req.Email)
	if err == nil {
		// No error means a row was found → email is taken.
		return nil, ErrEmailTaken
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Any other error is an unexpected DB problem.
		return nil, fmt.Errorf("checking email uniqueness: %w", err)
	}

	// bcrypt cost 12 is the OWASP-recommended minimum as of 2024.
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user := &User{
		Name:     req.Name,
		Email:    req.Email,
		Password: string(hashed),
		IsActive: true,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	slog.Info("New user registered", "user_id", user.ID, "email", user.Email)
	return toUserResponse(user), nil
}

// Login authenticates a user and returns a JWT access token + refresh token pair.
//
// Flow:
//  1. Find user by email (using ErrInvalidCreds for any failure to prevent enumeration).
//  2. Compare bcrypt hash.
//  3. Check active status.
//  4. Issue a signed JWT access token with a random JTI.
//  5. Store an opaque refresh token UUID in Redis (TTL = RefreshTTL from config).
func (s *service) Login(ctx context.Context, req LoginRequest) (*TokenPair, error) {
	// Fetch user — deliberately use ErrInvalidCreds regardless of what went wrong
	// (record not found vs. DB error) to prevent email enumeration.
	user, err := s.repo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, ErrInvalidCreds
	}

	// Constant-time comparison via bcrypt — prevents timing attacks.
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCreds
	}

	// Check active status after credential verification to avoid leaking which
	// accounts exist (an attacker would still get ErrInvalidCreds for bad passwords
	// on deactivated accounts before reaching this branch).
	if !user.IsActive {
		return nil, ErrInactiveUser
	}

	// Each token gets a unique JTI (JWT ID) used to blacklist it on logout.
	jti := uuid.NewString()
	accessToken, err := s.generateAccessToken(user, jti)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	// Refresh token is an opaque random UUID stored in Redis (not a JWT).
	// Storing it in Redis means we can revoke it server-side without any state in the token.
	refreshToken := uuid.NewString()
	if err := s.storeRefreshToken(ctx, refreshToken, user.ID); err != nil {
		// Non-fatal in development (Redis may not be running).
		// Fatal in production — without Redis, refresh tokens can't be revoked.
		slog.Warn("Could not store refresh token in Redis", "error", err)
		if s.cfg.App.Env != "development" {
			return nil, fmt.Errorf("storing refresh token: %w", err)
		}
	}

	slog.Info("User logged in", "user_id", user.ID)
	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.cfg.JWT.AccessTTL.Seconds()),
	}, nil
}

// Refresh validates the given refresh token in Redis and issues a new access token.
// The refresh token itself is NOT rotated — it remains valid until its TTL expires
// or the user explicitly logs out.
func (s *service) Refresh(ctx context.Context, refreshToken string) (*AccessTokenResponse, error) {
	if s.redis == nil {
		// Without Redis we cannot verify that the refresh token is valid.
		return nil, ErrInvalidToken
	}

	// Redis key format: "refresh:<uuid>"  →  value: "<user_id>"
	key := fmt.Sprintf("refresh:%s", refreshToken)
	val, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		// redis.Nil means the key doesn't exist (expired or never stored).
		return nil, ErrInvalidToken
	}

	// Parse the stored user ID.
	var userID uint
	if _, err := fmt.Sscanf(val, "%d", &userID); err != nil {
		return nil, ErrInvalidToken
	}

	// Verify the user still exists and is active before issuing a new token.
	user, err := s.repo.FindByID(ctx, userID)
	if err != nil || !user.IsActive {
		return nil, ErrInvalidToken
	}

	// New JTI for the new access token — each token can be independently revoked.
	jti := uuid.NewString()
	accessToken, err := s.generateAccessToken(user, jti)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	return &AccessTokenResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(s.cfg.JWT.AccessTTL.Seconds()),
	}, nil
}

// Logout revokes the current access token and (optionally) the refresh token.
//
// Access-token revocation:
//   The JTI is added to a Redis blacklist with a TTL equal to the token's
//   remaining validity. Once the token would have expired naturally, the
//   blacklist entry is also gone — bounding Redis memory growth.
//
// Refresh-token revocation:
//   The refresh token key is deleted from Redis immediately.
func (s *service) Logout(ctx context.Context, jti string, expiry time.Time, refreshToken string) error {
	if s.redis == nil {
		// Without Redis we cannot enforce revocation; warn and continue.
		slog.Warn("Logout called but Redis is unavailable — token not blacklisted")
		return nil
	}

	// Blacklist the access token JTI for the remainder of its natural lifetime.
	remaining := time.Until(expiry)
	if remaining > 0 && jti != "" {
		blacklistKey := fmt.Sprintf("blacklist:%s", jti)
		if err := s.redis.Set(ctx, blacklistKey, "1", remaining).Err(); err != nil {
			// Log but don't fail the request — the access token will expire naturally.
			slog.Error("Failed to blacklist token JTI", "jti", jti, "error", err)
		}
	}

	// Delete the refresh token so it cannot be exchanged for new access tokens.
	if refreshToken != "" {
		refreshKey := fmt.Sprintf("refresh:%s", refreshToken)
		s.redis.Del(ctx, refreshKey) // best-effort; ignore error
	}

	return nil
}

// ── Private helpers ────────────────────────────────────────────────────────────

// generateAccessToken creates a signed HS256 JWT access token for the given user.
// The jti parameter must be a unique string (uuid.NewString()) per call.
func (s *service) generateAccessToken(user *User, jti string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,                                        // unique token ID for blacklisting
			Subject:   fmt.Sprintf("%d", user.ID),                // standard "sub" claim
			IssuedAt:  jwt.NewNumericDate(now),                    // iat
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.JWT.AccessTTL)), // exp
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWT.Secret))
}

// storeRefreshToken persists a refresh-token UUID → user_id mapping in Redis
// with a TTL equal to the configured RefreshTTL (e.g. 7 days).
// Returns nil immediately when redis is nil (dev mode without Redis).
func (s *service) storeRefreshToken(ctx context.Context, token string, userID uint) error {
	if s.redis == nil {
		return nil
	}
	key := fmt.Sprintf("refresh:%s", token)
	value := fmt.Sprintf("%d", userID)
	return s.redis.Set(ctx, key, value, s.cfg.JWT.RefreshTTL).Err()
}
