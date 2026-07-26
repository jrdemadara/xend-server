package relationship

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"xend.chat/m/internal/objectstore"
)

const maxSpaceImageBytes = 8 << 20

var errUnsupportedSpaceImageType = errors.New("unsupported image type")

type MediaStore struct {
	store objectstore.Store
}

func NewMediaStore(store objectstore.Store) *MediaStore {
	return &MediaStore{store: store}
}

func (s *MediaStore) SaveSpaceImage(ctx context.Context, spaceID, kind string, file multipart.File) (string, error) {
	spaceID = strings.TrimSpace(spaceID)
	kind = strings.TrimSpace(kind)
	if spaceID == "" || kind == "" {
		return "", ErrInvalidInput
	}

	limitedReader := io.LimitReader(file, maxSpaceImageBytes+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrImageRequired
	}
	if len(data) > maxSpaceImageBytes {
		return "", fmt.Errorf("image exceeds %d MB", maxSpaceImageBytes/(1024*1024))
	}

	detectedType := http.DetectContentType(data)
	extension, ok := imageExtensionForType(detectedType)
	if !ok {
		return "", errUnsupportedSpaceImageType
	}

	fileName := fmt.Sprintf("%s-%s%s", time.Now().Format("20060102-150405"), uuid.NewString(), extension)
	relativePath := path.Join("spaces", spaceID, kind, fileName)
	if err := s.store.Put(ctx, relativePath, data, detectedType); err != nil {
		return "", err
	}
	return relativePath, nil
}

func (s *MediaStore) Delete(ctx context.Context, relativePath string) error {
	if strings.TrimSpace(relativePath) == "" {
		return nil
	}
	return s.store.Delete(ctx, relativePath)
}

func (s *MediaStore) ReadImage(ctx context.Context, relativePath string) ([]byte, string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return nil, "", ErrImageRequired
	}
	data, contentType, err := s.store.Get(ctx, relativePath)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
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
