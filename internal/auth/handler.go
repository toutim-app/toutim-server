package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/fabienbmusic/toutim-server/internal/database"
	"github.com/fabienbmusic/toutim-server/internal/model"
	"github.com/fabienbmusic/toutim-server/internal/util"
	"github.com/google/uuid"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Handler handles authentication endpoints.
type Handler struct {
	dbMgr  *database.DatabaseManager
	jwtMgr *JWTManager
	logger *slog.Logger
}

// NewHandler creates a new auth handler.
func NewHandler(dbMgr *database.DatabaseManager, jwtMgr *JWTManager, logger *slog.Logger) *Handler {
	return &Handler{
		dbMgr:  dbMgr,
		jwtMgr: jwtMgr,
		logger: logger,
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
	ID    string `json:"id"`
	Email string `json:"email"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
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

	h.logger.Info("user registered", "user_id", userID, "email", req.Email)

	util.RespondJSON(w, http.StatusCreated, authResponse{
		User:         &userResponse{ID: userID, Email: req.Email},
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
		User:         &userResponse{ID: user.ID, Email: user.Email},
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
	tokenHash := hashToken(req.RefreshToken)
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

	tokenHash := hashToken(req.RefreshToken)
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

// storeRefreshToken hashes and stores a refresh token.
func (h *Handler) storeRefreshToken(usersDB *sql.DB, userID, jti, rawToken string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	token := model.RefreshToken{
		ID:        jti,
		UserID:    userID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: time.Now().UTC().Add(h.jwtMgr.refreshTokenTTL).Format(time.RFC3339),
		CreatedAt: now,
	}
	return database.StoreRefreshToken(usersDB, token)
}

// hashToken returns the SHA-256 hex hash of a token string.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
