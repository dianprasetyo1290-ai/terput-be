package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalStorage struct {
	rootDir string
}

func NewLocalStorage(rootDir string) (*LocalStorage, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage root: %w", err)
	}
	return &LocalStorage{rootDir: rootDir}, nil
}

// Save streams content from r to a file stored at objectKey under rootDir,
// computing and returning the SHA256 hex checksum and byte size.
func (s *LocalStorage) Save(objectKey string, r io.Reader) (string, int64, error) {
	targetPath := filepath.Join(s.rootDir, objectKey)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create dir: %w", err)
	}

	f, err := os.Create(targetPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	written, err := io.Copy(mw, r)
	if err != nil {
		return "", 0, fmt.Errorf("failed to write content: %w", err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	return checksum, written, nil
}

// Open opens an existing object key for reading.
func (s *LocalStorage) Open(objectKey string) (*os.File, error) {
	targetPath := filepath.Join(s.rootDir, objectKey)
	return os.Open(targetPath)
}

// Exists checks if object key exists.
func (s *LocalStorage) Exists(objectKey string) bool {
	targetPath := filepath.Join(s.rootDir, objectKey)
	_, err := os.Stat(targetPath)
	return err == nil
}
