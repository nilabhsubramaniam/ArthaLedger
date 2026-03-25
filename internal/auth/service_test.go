package auth_test

// Unit tests for the auth service layer.
//
// These tests exercise all business-logic branches without a real database or
// Redis instance.  The test fixtures are:
//   - mockAuthRepo  — in-memory implementation of auth.Repository
//   - nil Redis     — auth.NewService accepts nil; the service degrades gracefully
//     when cfg.App.Env == "development" (no refresh-token revocation)
//   - testAuthCfg   — a minimal config.Config with a deterministic JWT secret

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nilabh/arthaledger/config"
	"github.com/nilabh/arthaledger/internal/auth"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ── Shared test config ────────────────────────────────────────────────────────

// testAuthCfg is a deterministic config used by all auth tests.
// The JWT secret must be at least 32 bytes for HS256 security.
var testAuthCfg = &config.Config{
	App: config.AppConfig{Env: "development"}, // nil Redis is tolerated in dev
	JWT: config.JWTConfig{
		Secret:     "super-secret-key-used-only-in-tests-1234",
		AccessTTL:  time.Hour,
		RefreshTTL: 7 * 24 * time.Hour,
	},
}

// ── Mock repository ───────────────────────────────────────────────────────────

// mockAuthRepo is a test double for auth.Repository.
// Each method stub is either set as a function field for per-test customisation,
// or falls back to a safe default (return nil/not-found).
type mockAuthRepo struct {
	findByEmailFn func(ctx context.Context, email string) (*auth.User, error)
	findByIDFn    func(ctx context.Context, id uint) (*auth.User, error)
	createFn      func(ctx context.Context, user *auth.User) error
}

func (m *mockAuthRepo) FindByEmail(ctx context.Context, email string) (*auth.User, error) {
	if m.findByEmailFn != nil {
		return m.findByEmailFn(ctx, email)
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockAuthRepo) FindByID(ctx context.Context, id uint) (*auth.User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockAuthRepo) Create(ctx context.Context, user *auth.User) error {
	if m.createFn != nil {
		return m.createFn(ctx, user)
	}
	// Simulate the DB auto-assigning an ID.
	user.ID = 1
	return nil
}

// ── Helper ────────────────────────────────────────────────────────────────────

// hashedPw returns a bcrypt hash that satisfies bcrypt.CompareHashAndPassword.
func hashedPw(plain string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	return string(h)
}

// ── Register tests ────────────────────────────────────────────────────────────

func TestAuthService_Register_Success(t *testing.T) {
	t.Parallel()
	repo := &mockAuthRepo{
		// FindByEmail returns not-found, meaning the email is available.
		findByEmailFn: func(_ context.Context, _ string) (*auth.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := auth.NewService(repo, nil, testAuthCfg)

	resp, err := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Alice",
		Email:    "alice@example.com",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", resp.Email)
	}
	if resp.ID == 0 {
		t.Error("expected a non-zero user ID")
	}
}

func TestAuthService_Register_EmailTaken(t *testing.T) {
	t.Parallel()
	repo := &mockAuthRepo{
		// FindByEmail returns a user, signalling the email is already in use.
		findByEmailFn: func(_ context.Context, _ string) (*auth.User, error) {
			return &auth.User{ID: 99, Email: "alice@example.com", IsActive: true}, nil
		},
	}
	svc := auth.NewService(repo, nil, testAuthCfg)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Name:     "Alice2",
		Email:    "alice@example.com",
		Password: "password123",
	})
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("expected ErrEmailTaken, got %v", err)
	}
}

// ── Login tests ───────────────────────────────────────────────────────────────

func TestAuthService_Login_Success(t *testing.T) {
	t.Parallel()
	const plainPw = "correct-password"
	repo := &mockAuthRepo{
		findByEmailFn: func(_ context.Context, _ string) (*auth.User, error) {
			return &auth.User{
				ID:       7,
				Email:    "bob@example.com",
				Password: hashedPw(plainPw),
				IsActive: true,
			}, nil
		},
	}
	svc := auth.NewService(repo, nil, testAuthCfg)

	pair, err := svc.Login(context.Background(), auth.LoginRequest{
		Email:    "bob@example.com",
		Password: plainPw,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("expected a non-empty access token")
	}
	if pair.ExpiresIn <= 0 {
		t.Error("expected positive expires_in")
	}
}

func TestAuthService_Login_UserNotFound(t *testing.T) {
	t.Parallel()
	repo := &mockAuthRepo{
		// Simulate DB error (not-found or otherwise) → ErrInvalidCreds expected.
		findByEmailFn: func(_ context.Context, _ string) (*auth.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := auth.NewService(repo, nil, testAuthCfg)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Email:    "nobody@example.com",
		Password: "anything",
	})
	if !errors.Is(err, auth.ErrInvalidCreds) {
		t.Errorf("expected ErrInvalidCreds, got %v", err)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	t.Parallel()
	repo := &mockAuthRepo{
		findByEmailFn: func(_ context.Context, _ string) (*auth.User, error) {
			return &auth.User{
				ID:       7,
				Email:    "bob@example.com",
				Password: hashedPw("correct-password"),
				IsActive: true,
			}, nil
		},
	}
	svc := auth.NewService(repo, nil, testAuthCfg)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Email:    "bob@example.com",
		Password: "wrong-password",
	})
	if !errors.Is(err, auth.ErrInvalidCreds) {
		t.Errorf("expected ErrInvalidCreds, got %v", err)
	}
}

func TestAuthService_Login_InactiveUser(t *testing.T) {
	t.Parallel()
	const plainPw = "correct-password"
	repo := &mockAuthRepo{
		findByEmailFn: func(_ context.Context, _ string) (*auth.User, error) {
			return &auth.User{
				ID:       8,
				Email:    "carol@example.com",
				Password: hashedPw(plainPw),
				IsActive: false, // deactivated account
			}, nil
		},
	}
	svc := auth.NewService(repo, nil, testAuthCfg)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Email:    "carol@example.com",
		Password: plainPw,
	})
	if !errors.Is(err, auth.ErrInactiveUser) {
		t.Errorf("expected ErrInactiveUser, got %v", err)
	}
}

// ── Refresh tests ─────────────────────────────────────────────────────────────

func TestAuthService_Refresh_NilRedis(t *testing.T) {
	t.Parallel()
	svc := auth.NewService(&mockAuthRepo{}, nil, testAuthCfg)

	// Without Redis, Refresh must always return ErrInvalidToken.
	_, err := svc.Refresh(context.Background(), "any-refresh-token")
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken when Redis is nil, got %v", err)
	}
}

// ── Logout tests ──────────────────────────────────────────────────────────────

func TestAuthService_Logout_NilRedis(t *testing.T) {
	t.Parallel()
	svc := auth.NewService(&mockAuthRepo{}, nil, testAuthCfg)

	// Logout with nil Redis should succeed (no-op: nothing to blacklist).
	err := svc.Logout(context.Background(), "some-jti", time.Now().Add(time.Hour), "some-refresh-token")
	if err != nil {
		t.Errorf("expected no error from Logout (nil Redis), got: %v", err)
	}
}
