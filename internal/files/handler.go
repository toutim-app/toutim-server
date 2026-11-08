package files

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/toutim-app/toutim-server/internal/auth"
	"github.com/toutim-app/toutim-server/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var fileIDRegex = regexp.MustCompile(`^img_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Allowed MIME types and their extensions.
var allowedMimeTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// Handler handles file upload/download/delete endpoints.
type Handler struct {
	dataDir     string
	maxFileSize int64
	logger      *slog.Logger
}

// NewHandler creates a new file handler.
func NewHandler(dataDir string, maxFileSize int64, logger *slog.Logger) *Handler {
	return &Handler{
		dataDir:     dataDir,
		maxFileSize: maxFileSize,
		logger:      logger,
	}
}

// Upload handles POST /api/files/upload
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize)

	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		util.RespondError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("file too large (max %d bytes)", h.maxFileSize))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		util.RespondError(w, http.StatusBadRequest, "missing 'file' form field")
		return
	}
	defer file.Close()

	// Detect content type
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		util.RespondError(w, http.StatusInternalServerError, "failed to read file")
		return
	}
	contentType := http.DetectContentType(buf[:n])

	// Validate MIME type
	ext, ok := allowedMimeTypes[contentType]
	if !ok {
		util.RespondError(w, http.StatusBadRequest,
			fmt.Sprintf("unsupported file type: %s (allowed: jpeg, png, webp, gif)", contentType))
		return
	}

	// Reset file reader
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		util.RespondError(w, http.StatusInternalServerError, "failed to process file")
		return
	}

	// Generate file ID
	fileID := "img_" + uuid.New().String()

	// Create user files directory
	filesDir := filepath.Join(h.dataDir, "user_data", userID+"_files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		h.logger.Error("failed to create files directory", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Write file
	filePath := filepath.Join(filesDir, fileID+ext)
	dst, err := os.Create(filePath)
	if err != nil {
		h.logger.Error("failed to create file", "error", err)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		h.logger.Error("failed to write file", "error", err)
		os.Remove(filePath)
		util.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.logger.Info("file uploaded",
		"user_id", userID,
		"file_id", fileID,
		"size", written,
		"original_name", header.Filename,
	)

	util.RespondJSON(w, http.StatusCreated, map[string]any{
		"file_id": fileID,
		"size":    written,
	})
}

// Download handles GET /api/files/{fileID}
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	fileID := chi.URLParam(r, "fileID")

	if !fileIDRegex.MatchString(fileID) {
		util.RespondError(w, http.StatusBadRequest, "invalid file ID format")
		return
	}

	// Find the file (glob for extension)
	filesDir := filepath.Join(h.dataDir, "user_data", userID+"_files")
	matches, err := filepath.Glob(filepath.Join(filesDir, fileID+".*"))
	if err != nil || len(matches) == 0 {
		util.RespondError(w, http.StatusNotFound, "file not found")
		return
	}

	filePath := matches[0]
	ext := strings.ToLower(filepath.Ext(filePath))

	// Set content type based on extension
	contentTypes := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".webp": "image/webp",
		".gif":  "image/gif",
	}
	if ct, ok := contentTypes[ext]; ok {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")

	http.ServeFile(w, r, filePath)
}

// Delete handles DELETE /api/files/{fileID}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	fileID := chi.URLParam(r, "fileID")

	if !fileIDRegex.MatchString(fileID) {
		util.RespondError(w, http.StatusBadRequest, "invalid file ID format")
		return
	}

	// Find and delete the file
	filesDir := filepath.Join(h.dataDir, "user_data", userID+"_files")
	matches, err := filepath.Glob(filepath.Join(filesDir, fileID+".*"))
	if err != nil || len(matches) == 0 {
		util.RespondError(w, http.StatusNotFound, "file not found")
		return
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			h.logger.Error("failed to delete file", "error", err, "path", match)
		}
	}

	h.logger.Info("file deleted", "user_id", userID, "file_id", fileID)

	util.RespondJSON(w, http.StatusOK, map[string]string{"message": "file deleted"})
}
