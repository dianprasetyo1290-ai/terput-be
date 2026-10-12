package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Object struct {
	Key      string
	MIME     string
	Checksum string
	Size     int64
}

type Storage interface {
	Put(ctx context.Context, key, mime string, r io.Reader, size int64) (Object, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

type Local struct {
	Root    string
	Max     int64
	Allowed map[string]bool
}

func NewLocal(root string, max int64) *Local {
	return &Local{
		Root: root,
		Max:  max,
		Allowed: map[string]bool{
			"application/pdf": true,
			"image/jpeg":      true,
			"image/png":       true,
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": true,
			"text/csv":                 true,
			"application/octet-stream": true,
		},
	}
}

// NewLocalStorage returns a new Local storage with default 15MB limit.
func NewLocalStorage(root string) *Local {
	return NewLocal(root, 15<<20)
}

func (l *Local) path(key string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(key, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid object key")
	}
	return filepath.Join(l.Root, clean), nil
}

func (l *Local) Put(_ context.Context, key, mime string, r io.Reader, size int64) (Object, error) {
	if !l.Allowed[mime] {
		return Object{}, errors.New("mime type not allowed")
	}
	if size <= 0 || size > l.Max {
		return Object{}, errors.New("invalid object size")
	}
	p, err := l.path(key)
	if err != nil {
		return Object{}, err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return Object{}, err
	}
	tmp := p + ".partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Object{}, err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, l.Max+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n != size || n > l.Max {
		_ = os.Remove(tmp)
		return Object{}, errors.New("object size mismatch")
	}
	if err = os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return Object{}, err
	}
	return Object{Key: key, MIME: mime, Checksum: hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
