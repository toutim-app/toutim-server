package auth

import (
	"database/sql"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/fabienbmusic/toutim-server/internal/config"
	"github.com/fabienbmusic/toutim-server/internal/database"
	"github.com/fabienbmusic/toutim-server/internal/email"
	"github.com/fabienbmusic/toutim-server/internal/model"
	"github.com/fabienbmusic/toutim-server/internal/util"
	"github.com/google/uuid"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Handler handles authentication endpoints.
type Handler struct {
	dbMgr       *database.DatabaseManager
	jwtMgr      *JWTManager
	emailSender *email.Sender
	smtpCfg     config.SMTPConfig
	logger      *slog.Logger
}

// NewHandler creates a new auth handler.
func NewHandler(dbMgr *database.DatabaseManager, jwtMgr *JWTManager, emailSender *email.Sender, smtpCfg config.SMTPConfig, logger *slog.Logger) *Handler {
	return &Handler{
		dbMgr:       dbMgr,
		jwtMgr:      jwtMgr,
		emailSender: emailSender,
		smtpCfg:     smtpCfg,
		logger:      logger,
	}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	User         *userResponse `json:"user"`
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token"`
}

type userResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

// Register handles POST /api/auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}

	// Validate email
	if !emailRegex.MatchString(req.Email) {
		util.RespondError(w, http.StatusBadRequest, "invalid email format")
		return
	}

	// Validate password
	if len(req.Password) < minPasswordLen {
		util.RespondError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	usersDB := h.dbMgr.UsersDB()

	// Check if email already exists
	existing, err := database.GetUserByEmail(usersDB, req.Email)
	if err != nil {
		h.logger.Error("failed to check existing user", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		util.RespondError(w, http.StatusConflict, "email already registered")
		return
	}

	// Hash password
	hash, err := HashPassword(req.Password)
	if err != nil {
		h.logger.Error("failed to hash password", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Create user
	now := time.Now().UTC().Format(time.RFC3339)
	userID := uuid.New().String()
	user := model.User{
		ID:           userID,
		Email:        req.Email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
		IsActive:     true,
	}

	if err := database.CreateUser(usersDB, user); err != nil {
		h.logger.Error("failed to create user", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Initialize user database (creates the file and applies migrations)
	if _, err := h.dbMgr.UserDB(userID); err != nil {
		h.logger.Error("failed to create user database", "error", err, "user_id", userID)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Generate tokens
	accessToken, err := h.jwtMgr.GenerateAccessToken(userID, req.Email)
	if err != nil {
		h.logger.Error("failed to generate access token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	refreshToken, jti, err := h.jwtMgr.GenerateRefreshToken(userID)
	if err != nil {
		h.logger.Error("failed to generate refresh token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Store refresh token hash
	if err := h.storeRefreshToken(usersDB, userID, jti, refreshToken); err != nil {
		h.logger.Error("failed to store refresh token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Handle email verification
	emailVerified := false
	if h.emailSender.Enabled() {
		rawToken, tokenHash, err := email.GenerateToken()
		if err != nil {
			h.logger.Error("failed to generate verification token", "error", err)
		} else {
			vToken := model.EmailVerificationToken{
				ID:        uuid.New().String(),
				UserID:    userID,
				TokenHash: tokenHash,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
				CreatedAt: now,
			}
			if err := database.StoreEmailVerificationToken(usersDB, vToken); err != nil {
				h.logger.Error("failed to store verification token", "error", err)
			} else {
				subject, body, err := email.RenderVerificationEmail(h.smtpCfg.BaseURL, rawToken)
				if err != nil {
					h.logger.Error("failed to render verification email", "error", err)
				} else {
					go func() {
						if err := h.emailSender.Send(req.Email, subject, body); err != nil {
							h.logger.Error("failed to send verification email", "error", err, "email", req.Email)
						} else {
							h.logger.Info("verification email sent", "email", req.Email)
						}
					}()
				}
			}
		}
	} else {
		// No SMTP configured: auto-verify user
		if err := database.SetEmailVerified(usersDB, userID, now); err != nil {
			h.logger.Error("failed to auto-verify user", "error", err)
		} else {
			emailVerified = true
		}
	}

	h.logger.Info("user registered", "user_id", userID, "email", req.Email)

	util.RespondJSON(w, http.StatusCreated, authResponse{
		User:         &userResponse{ID: userID, Email: req.Email, EmailVerified: emailVerified},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// Login handles POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}

	usersDB := h.dbMgr.UsersDB()

	user, err := database.GetUserByEmail(usersDB, req.Email)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		util.RespondError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if !user.IsActive {
		util.RespondError(w, http.StatusForbidden, "account is deactivated")
		return
	}

	ok, err := VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		h.logger.Error("failed to verify password", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		util.RespondError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Generate tokens
	accessToken, err := h.jwtMgr.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		h.logger.Error("failed to generate access token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	refreshToken, jti, err := h.jwtMgr.GenerateRefreshToken(user.ID)
	if err != nil {
		h.logger.Error("failed to generate refresh token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.storeRefreshToken(usersDB, user.ID, jti, refreshToken); err != nil {
		h.logger.Error("failed to store refresh token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("user logged in", "user_id", user.ID)

	util.RespondJSON(w, http.StatusOK, authResponse{
		User:         &userResponse{ID: user.ID, Email: user.Email, EmailVerified: user.EmailVerified},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// Refresh handles POST /api/auth/refresh
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}

	if req.RefreshToken == "" {
		util.RespondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	// Validate the refresh token JWT
	claims, err := h.jwtMgr.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		util.RespondError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	usersDB := h.dbMgr.UsersDB()

	// Check token exists in database (not revoked)
	tokenHash := util.HashToken(req.RefreshToken)
	storedToken, err := database.GetRefreshToken(usersDB, tokenHash)
	if err != nil {
		h.logger.Error("failed to get refresh token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if storedToken == nil {
		util.RespondError(w, http.StatusUnauthorized, "refresh token has been revoked")
		return
	}

	// Delete old token (rotation)
	if err := database.DeleteRefreshToken(usersDB, tokenHash); err != nil {
		h.logger.Error("failed to delete old refresh token", "error", err)
	}

	// Get user for email
	user, err := database.GetUserByID(usersDB, claims.Subject)
	if err != nil || user == nil {
		util.RespondError(w, http.StatusUnauthorized, "user not found")
		return
	}

	// Generate new tokens
	accessToken, err := h.jwtMgr.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		h.logger.Error("failed to generate access token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	refreshToken, jti, err := h.jwtMgr.GenerateRefreshToken(user.ID)
	if err != nil {
		h.logger.Error("failed to generate refresh token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.storeRefreshToken(usersDB, user.ID, jti, refreshToken); err != nil {
		h.logger.Error("failed to store refresh token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	util.RespondJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// Logout handles POST /api/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}

	if req.RefreshToken == "" {
		util.RespondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	tokenHash := util.HashToken(req.RefreshToken)
	if err := database.DeleteRefreshToken(h.dbMgr.UsersDB(), tokenHash); err != nil {
		h.logger.Error("failed to delete refresh token", "error", err)
	}

	util.RespondJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

// DeleteAccount handles DELETE /api/auth/account
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	if userID == "" {
		util.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	usersDB := h.dbMgr.UsersDB()

	// Delete all refresh tokens
	if err := database.DeleteRefreshTokensByUser(usersDB, userID); err != nil {
		h.logger.Error("failed to delete refresh tokens", "error", err, "user_id", userID)
	}

	// Remove user database and files
	if err := h.dbMgr.RemoveUserDB(userID); err != nil {
		h.logger.Error("failed to remove user database", "error", err, "user_id", userID)
	}

	// Delete user record
	if err := database.DeleteUser(usersDB, userID); err != nil {
		h.logger.Error("failed to delete user", "error", err, "user_id", userID)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("account deleted", "user_id", userID)

	util.RespondJSON(w, http.StatusOK, map[string]string{"message": "account deleted"})
}

// VerifyEmail handles POST /api/auth/verify-email
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}
	if req.Token == "" {
		util.RespondError(w, http.StatusBadRequest, "token is required")
		return
	}

	usersDB := h.dbMgr.UsersDB()
	tokenHash := util.HashToken(req.Token)

	storedToken, err := database.GetEmailVerificationToken(usersDB, tokenHash)
	if err != nil {
		h.logger.Error("failed to get verification token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if storedToken == nil {
		util.RespondError(w, http.StatusBadRequest, "invalid or expired verification token")
		return
	}

	expiresAt, err := time.Parse(time.RFC3339, storedToken.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		database.DeleteEmailVerificationToken(usersDB, tokenHash)
		util.RespondError(w, http.StatusBadRequest, "verification token has expired")
		return
	}

	now := util.NowRFC3339()
	if err := database.SetEmailVerified(usersDB, storedToken.UserID, now); err != nil {
		h.logger.Error("failed to verify email", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	database.DeleteEmailVerificationTokensByUser(usersDB, storedToken.UserID)

	h.logger.Info("email verified", "user_id", storedToken.UserID)
	util.RespondJSON(w, http.StatusOK, map[string]string{"message": "email verified successfully"})
}

// ResendVerification handles POST /api/auth/resend-verification
func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	if !h.emailSender.Enabled() {
		util.RespondError(w, http.StatusServiceUnavailable, "email service not configured")
		return
	}

	var req resendVerificationRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}
	if !emailRegex.MatchString(req.Email) {
		util.RespondError(w, http.StatusBadRequest, "invalid email format")
		return
	}

	usersDB := h.dbMgr.UsersDB()

	user, err := database.GetUserByEmail(usersDB, req.Email)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Always return success to avoid email enumeration
	if user == nil || user.EmailVerified {
		util.RespondJSON(w, http.StatusOK, map[string]string{
			"message": "if the email exists and is not verified, a verification email has been sent",
		})
		return
	}

	database.DeleteEmailVerificationTokensByUser(usersDB, user.ID)

	rawToken, tokenHash, err := email.GenerateToken()
	if err != nil {
		h.logger.Error("failed to generate verification token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := util.NowRFC3339()
	vToken := model.EmailVerificationToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		CreatedAt: now,
	}
	if err := database.StoreEmailVerificationToken(usersDB, vToken); err != nil {
		h.logger.Error("failed to store verification token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	subject, body, err := email.RenderVerificationEmail(h.smtpCfg.BaseURL, rawToken)
	if err != nil {
		h.logger.Error("failed to render verification email", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go func() {
		if err := h.emailSender.Send(req.Email, subject, body); err != nil {
			h.logger.Error("failed to send verification email", "error", err, "email", req.Email)
		}
	}()

	util.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "if the email exists and is not verified, a verification email has been sent",
	})
}

// ForgotPassword handles POST /api/auth/forgot-password
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if !h.emailSender.Enabled() {
		util.RespondError(w, http.StatusServiceUnavailable, "email service not configured")
		return
	}

	var req forgotPasswordRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}
	if !emailRegex.MatchString(req.Email) {
		util.RespondError(w, http.StatusBadRequest, "invalid email format")
		return
	}

	usersDB := h.dbMgr.UsersDB()

	user, err := database.GetUserByEmail(usersDB, req.Email)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Always return success to prevent email enumeration
	if user == nil || !user.IsActive {
		util.RespondJSON(w, http.StatusOK, map[string]string{
			"message": "if the email exists, a password reset link has been sent",
		})
		return
	}

	database.DeletePasswordResetTokensByUser(usersDB, user.ID)

	rawToken, tokenHash, err := email.GenerateToken()
	if err != nil {
		h.logger.Error("failed to generate reset token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	const resetTokenTTL = 30 * time.Minute

	now := util.NowRFC3339()
	rToken := model.PasswordResetToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(resetTokenTTL).Format(time.RFC3339),
		CreatedAt: now,
	}
	if err := database.StorePasswordResetToken(usersDB, rToken); err != nil {
		h.logger.Error("failed to store reset token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	subject, body, err := email.RenderPasswordResetEmail(h.smtpCfg.BaseURL, rawToken, 30)
	if err != nil {
		h.logger.Error("failed to render reset email", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go func() {
		if err := h.emailSender.Send(user.Email, subject, body); err != nil {
			h.logger.Error("failed to send reset email", "error", err, "email", user.Email)
		}
	}()

	h.logger.Info("password reset email sent", "user_id", user.ID)
	util.RespondJSON(w, http.StatusOK, map[string]string{
		"message": "if the email exists, a password reset link has been sent",
	})
}

// ResetPassword handles POST /api/auth/reset-password
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if !util.DecodeJSON(w, r, &req) {
		return
	}
	if req.Token == "" {
		util.RespondError(w, http.StatusBadRequest, "token is required")
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		util.RespondError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	usersDB := h.dbMgr.UsersDB()
	tokenHash := util.HashToken(req.Token)

	storedToken, err := database.GetPasswordResetToken(usersDB, tokenHash)
	if err != nil {
		h.logger.Error("failed to get reset token", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if storedToken == nil {
		util.RespondError(w, http.StatusBadRequest, "invalid or expired reset token")
		return
	}

	expiresAt, err := time.Parse(time.RFC3339, storedToken.ExpiresAt)
	if err != nil || time.Now().UTC().After(expiresAt) {
		database.DeletePasswordResetToken(usersDB, tokenHash)
		util.RespondError(w, http.StatusBadRequest, "reset token has expired")
		return
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		h.logger.Error("failed to hash password", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := util.NowRFC3339()
	if err := database.UpdatePasswordHash(usersDB, storedToken.UserID, hash, now); err != nil {
		h.logger.Error("failed to update password", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Delete all reset tokens and invalidate all sessions
	database.DeletePasswordResetTokensByUser(usersDB, storedToken.UserID)
	database.DeleteRefreshTokensByUser(usersDB, storedToken.UserID)

	h.logger.Info("password reset successful", "user_id", storedToken.UserID)
	util.RespondJSON(w, http.StatusOK, map[string]string{"message": "password reset successfully"})
}

// storeRefreshToken hashes and stores a refresh token.
func (h *Handler) storeRefreshToken(usersDB *sql.DB, userID, jti, rawToken string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	token := model.RefreshToken{
		ID:        jti,
		UserID:    userID,
		TokenHash: util.HashToken(rawToken),
		ExpiresAt: time.Now().UTC().Add(h.jwtMgr.refreshTokenTTL).Format(time.RFC3339),
		CreatedAt: now,
	}
	return database.StoreRefreshToken(usersDB, token)
}
