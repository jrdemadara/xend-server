package dailyritual

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxImageBytes = 8 << 20

var errUnsupportedImageType = errors.New("unsupported image type")

type SubmissionStore struct {
	rootDir string
}

func NewSubmissionStore(rootDir string) *SubmissionStore {
	return &SubmissionStore{rootDir: rootDir}
}

func (s *SubmissionStore) SaveImage(file multipart.File) (string, error) {
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return "", err
	}

	limitedReader := io.LimitReader(file, maxImageBytes+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrImageRequired
	}
	if len(data) > maxImageBytes {
		return "", fmt.Errorf("image exceeds %d MB", maxImageBytes/(1024*1024))
	}

	detectedType := http.DetectContentType(data)
	extension, ok := imageExtensionForType(detectedType)
	if !ok {
		return "", errUnsupportedImageType
	}

	fileName := fmt.Sprintf("%s-%s%s", time.Now().Format("20060102-150405"), uuid.NewString(), extension)
	relativePath := filepath.ToSlash(filepath.Join("daily-rituals", fileName))
	absolutePath := filepath.Join(s.rootDir, fileName)
	if err := os.WriteFile(absolutePath, data, 0o644); err != nil {
		return "", err
	}
	return relativePath, nil
}

func (s *SubmissionStore) Delete(relativePath string) error {
	if strings.TrimSpace(relativePath) == "" {
		return nil
	}
	fileName := filepath.Base(relativePath)
	if fileName == "." || fileName == string(filepath.Separator) || fileName == "" {
		return nil
	}
	err := os.Remove(filepath.Join(s.rootDir, fileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func imageExtensionForType(contentType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/webp":
		return ".webp", true
	case "image/heic":
		return ".heic", true
	case "image/heif":
		return ".heif", true
	default:
		return "", false
	}
}
