package objectstore

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

type LocalStore struct {
	rootDir string
}

func NewLocalStore(rootDir string) *LocalStore {
	return &LocalStore{rootDir: rootDir}
}

func (s *LocalStore) Put(_ context.Context, key string, data []byte, _ string) error {
	cleanedKey, err := cleanKey(key)
	if err != nil {
		return err
	}

	absolutePath := filepath.Join(s.rootDir, filepath.FromSlash(cleanedKey))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absolutePath, data, 0o644)
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	cleanedKey, err := cleanKey(key)
	if err != nil {
		return nil
	}

	err = os.Remove(filepath.Join(s.rootDir, filepath.FromSlash(cleanedKey)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *LocalStore) Get(_ context.Context, key string) ([]byte, string, error) {
	cleanedKey, err := cleanKey(key)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(filepath.Join(s.rootDir, filepath.FromSlash(cleanedKey)))
	if err != nil {
		return nil, "", err
	}
	return data, http.DetectContentType(data), nil
}
