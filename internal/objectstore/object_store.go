package objectstore

import (
	"context"
	"errors"
	"path"
	"strings"
)

var ErrInvalidKey = errors.New("invalid object key")

type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Delete(ctx context.Context, key string) error
	Get(ctx context.Context, key string) ([]byte, string, error)
}

func cleanKey(key string) (string, error) {
	cleaned := strings.Trim(path.Clean("/"+strings.TrimSpace(key)), "/")
	if cleaned == "" || cleaned == "." {
		return "", ErrInvalidKey
	}
	return cleaned, nil
}
