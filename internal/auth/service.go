package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pgxconn "github.com/jackc/pgx/v5/pgconn"
	"xend.chat/m/pkg/idgen"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrDeviceNotFound     = errors.New("device not found")
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidToken       = errors.New("invalid token")
	ErrEmailExists        = errors.New("email already exists")
	ErrEmailNotVerified   = errors.New("email is not verified")
	ErrLoginRateLimited   = errors.New("login rate limited")
	ErrVerifyRateLimited  = errors.New("verification rate limited")
)

type Service struct {
	repo             *Repository
	tokens           *TokenManager
	googleVerifier   GoogleVerifier
	emailVerifyStore EmailVerificationStore
	loginAttempts    LoginAttemptStore
	emailEnqueuer    VerificationEmailEnqueuer
	emailVerifyTTL   time.Duration
}

func NewService(repo *Repository, tokens *TokenManager, googleVerifier GoogleVerifier, emailVerifyStore EmailVerificationStore, loginAttempts LoginAttemptStore, emailEnqueuer VerificationEmailEnqueuer) *Service {
	return &Service{
		repo:             repo,
		tokens:           tokens,
		googleVerifier:   googleVerifier,
		emailVerifyStore: emailVerifyStore,
		loginAttempts:    loginAttempts,
		emailEnqueuer:    emailEnqueuer,
		emailVerifyTTL:   24 * time.Hour,
	}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest, clientIP string) (RegisterResponse, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	if req.Email == "" || len(req.Password) < 8 || req.DisplayName == "" || req.DeviceName == "" || req.Platform == "" || req.RegistrationID <= 0 || req.IdentityKeyPublic == "" {
		return RegisterResponse{}, ErrInvalidInput
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return RegisterResponse{}, err
	}

	var userID string
	for range 5 {
		identifier, genErr := idgen.Identifier(8)
		if genErr != nil {
			return RegisterResponse{}, genErr
		}
		userID, _, err = s.repo.CreateUserWithDevice(ctx, req, hash, identifier)
		if err == nil {
			break
		}
		if !isUniqueViolation(err) {
			return RegisterResponse{}, mapCreateErr(err)
		}
		if isEmailUniqueViolation(err) {
			u, getErr := s.repo.GetUserByEmail(ctx, req.Email)
			if getErr != nil {
				return RegisterResponse{}, ErrEmailExists
			}
			if u.EmailVerifiedAt == nil {
				if resendErr := s.createEmailVerification(ctx, u.ID, req.Email, clientIP); resendErr != nil && !errors.Is(resendErr, ErrVerifyRateLimited) {
					return RegisterResponse{}, resendErr
				}
				return RegisterResponse{UserID: u.ID, Email: req.Email, RequiresVerification: true}, nil
			}
			return RegisterResponse{}, ErrEmailExists
		}
	}
	if err != nil {
		return RegisterResponse{}, mapCreateErr(err)
	}

	if err := s.createEmailVerification(ctx, userID, req.Email, clientIP); err != nil {
		return RegisterResponse{}, err
	}
	return RegisterResponse{UserID: userID, Email: req.Email, RequiresVerification: true}, nil
}

func (s *Service) VerifyEmail(ctx context.Context, req VerifyEmailRequest) error {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Token = strings.ToUpper(strings.TrimSpace(req.Token))
	if req.Email == "" || req.Token == "" {
		return ErrInvalidInput
	}
	consumed, err := s.emailVerifyStore.Consume(ctx, req.Email, HashToken(req.Token))
	if err != nil {
		return err
	}
	if !consumed {
		return ErrInvalidToken
	}
	updated, err := s.repo.MarkEmailVerifiedByEmail(ctx, req.Email)
	if err != nil {
		return err
	}
	if !updated {
		return ErrInvalidToken
	}
	return nil
}

func (s *Service) ResendEmailVerification(ctx context.Context, req ResendVerificationRequest, clientIP string) error {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		return ErrInvalidInput
	}
	u, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if u.EmailVerifiedAt != nil {
		return nil
	}
	return s.createEmailVerification(ctx, u.ID, req.Email, clientIP)
}

func (s *Service) Login(ctx context.Context, req LoginRequest, clientIP string) (AuthResponse, error) {
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	if req.Email == "" || req.Password == "" || req.DeviceName == "" {
		return AuthResponse{}, ErrInvalidInput
	}
	if s.loginAttempts != nil {
		locked, lockErr := s.loginAttempts.IsLocked(ctx, req.Email, clientIP)
		if lockErr != nil {
			return AuthResponse{}, lockErr
		}
		if locked {
			return AuthResponse{}, ErrLoginRateLimited
		}
	}

	u, err := s.repo.GetUserForLogin(ctx, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if s.loginAttempts != nil {
				_ = s.loginAttempts.RegisterFailure(ctx, req.Email, clientIP)
			}
			return AuthResponse{}, ErrInvalidCredentials
		}
		return AuthResponse{}, err
	}
	if u.EmailVerifiedAt == nil {
		return AuthResponse{}, ErrEmailNotVerified
	}
	if err := VerifyPassword(u.PasswordHash, req.Password); err != nil {
		if s.loginAttempts != nil {
			_ = s.loginAttempts.RegisterFailure(ctx, req.Email, clientIP)
		}
		return AuthResponse{}, ErrInvalidCredentials
	}

	d, err := s.repo.GetActiveDeviceByName(ctx, u.ID, req.DeviceName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthResponse{}, ErrDeviceNotFound
		}
		return AuthResponse{}, err
	}
	if s.loginAttempts != nil {
		_ = s.loginAttempts.Clear(ctx, req.Email, clientIP)
	}
	return s.issueSession(ctx, u.ID, d.ID)
}

func (s *Service) GoogleSignIn(ctx context.Context, req GoogleAuthRequest) (AuthResponse, error) {
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if s.googleVerifier == nil || req.IDToken == "" || req.DeviceName == "" || req.Platform == "" || req.RegistrationID <= 0 || req.IdentityKeyPublic == "" {
		return AuthResponse{}, ErrInvalidInput
	}

	claims, err := s.googleVerifier.Verify(ctx, req.IDToken)
	if err != nil {
		return AuthResponse{}, ErrInvalidToken
	}
	if !claims.EmailVerified || claims.Email == "" {
		return AuthResponse{}, ErrEmailNotVerified
	}

	email := strings.ToLower(strings.TrimSpace(claims.Email))
	displayName := req.DisplayName
	if displayName == "" {
		displayName = strings.TrimSpace(claims.Name)
	}
	if displayName == "" {
		displayName = "Xend User"
	}

	user, err := s.repo.GetUserByOAuth(ctx, "google", claims.Subject)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, err
	}

	userID := user.ID
	if userID == "" {
		u, emailErr := s.repo.GetUserByEmail(ctx, email)
		if emailErr == nil {
			userID = u.ID
		} else if !errors.Is(emailErr, pgx.ErrNoRows) {
			return AuthResponse{}, emailErr
		}
	}

	if userID == "" {
		for range 5 {
			identifier, genErr := idgen.Identifier(8)
			if genErr != nil {
				return AuthResponse{}, genErr
			}
			userID, err = s.repo.CreateUserForOAuth(ctx, displayName, email, identifier)
			if err == nil {
				break
			}
			if !isUniqueViolation(err) {
				return AuthResponse{}, err
			}
		}
		if err != nil {
			return AuthResponse{}, err
		}
	}

	if err = s.repo.LinkOAuthAccount(ctx, userID, "google", claims.Subject, email); err != nil {
		return AuthResponse{}, err
	}

	d, err := s.repo.EnsureActiveDevice(ctx, userID, req.DeviceName, req.Platform, req.RegistrationID, req.IdentityKeyPublic)
	if err != nil {
		return AuthResponse{}, err
	}

	return s.issueSession(ctx, userID, d.ID)
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (TokenRefreshResponse, error) {
	userID, deviceID, sessionID, err := s.tokens.ParseRefreshToken(rawRefreshToken)
	if err != nil {
		return TokenRefreshResponse{}, ErrInvalidToken
	}

	newSessionID := uuid.NewString()
	newRawRefresh, newExp, err := s.tokens.CreateRefreshToken(userID, deviceID, newSessionID)
	if err != nil {
		return TokenRefreshResponse{}, err
	}
	_, err = s.repo.RotateRefreshSession(ctx, sessionID, newSessionID, HashToken(newRawRefresh), newExp)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TokenRefreshResponse{}, ErrInvalidToken
		}
		return TokenRefreshResponse{}, err
	}

	access, _, err := s.tokens.CreateAccessToken(userID, deviceID)
	if err != nil {
		return TokenRefreshResponse{}, err
	}
	return TokenRefreshResponse{AccessToken: access, RefreshToken: newRawRefresh, ExpiresIn: int64(s.tokens.accessTTL.Seconds())}, nil
}

func (s *Service) Logout(ctx context.Context, userID, deviceID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(deviceID) == "" {
		return ErrInvalidInput
	}
	return s.repo.RevokeRefreshSessionsByUserDevice(ctx, userID, deviceID)
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrInvalidInput
	}
	return s.repo.RevokeAllRefreshSessionsByUser(ctx, userID)
}

func (s *Service) issueSession(ctx context.Context, userID, deviceID string) (AuthResponse, error) {
	sessionID := uuid.NewString()
	refresh, refreshExp, err := s.tokens.CreateRefreshToken(userID, deviceID, sessionID)
	if err != nil {
		return AuthResponse{}, err
	}
	if err := s.repo.InsertRefreshSession(ctx, Session{ID: sessionID, UserID: userID, DeviceID: deviceID, RefreshTokenHash: HashToken(refresh), ExpiresAt: refreshExp}); err != nil {
		return AuthResponse{}, err
	}

	access, _, err := s.tokens.CreateAccessToken(userID, deviceID)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("create access token: %w", err)
	}
	return AuthResponse{UserID: userID, DeviceID: deviceID, AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(s.tokens.accessTTL.Seconds())}, nil
}

func (s *Service) createEmailVerification(ctx context.Context, userID, email, clientIP string) error {
	allowed, err := s.emailVerifyStore.AllowSend(ctx, email, clientIP)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrVerifyRateLimited
	}

	raw, hash, err := NewVerificationToken()
	if err != nil {
		return err
	}
	_ = userID
	if err := s.emailVerifyStore.Save(ctx, email, hash, s.emailVerifyTTL); err != nil {
		return err
	}
	if s.emailEnqueuer == nil {
		return fmt.Errorf("verification email enqueuer is not configured")
	}
	return s.emailEnqueuer.EnqueueVerificationEmail(ctx, email, raw)
}

func mapCreateErr(err error) error {
	if isEmailUniqueViolation(err) {
		return ErrEmailExists
	}
	return err
}

func isEmailUniqueViolation(err error) bool {
	pgErr := &pgxconn.PgError{}
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key"
}

func isUniqueViolation(err error) bool {
	pgErr := &pgxconn.PgError{}
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
