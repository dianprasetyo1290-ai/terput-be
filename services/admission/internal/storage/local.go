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
	Key, MIME, Checksum string
	Size                int64
}
type Storage interface {
	Put(context.Context, string, string, io.Reader, int64) (Object, error)
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Local struct {
	Root    string
	Max     int64
	Allowed map[string]bool
}

func NewLocal(root string, max int64) *Local {
	return &Local{root, max, map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true}}
}
func (l *Local) path(key string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(key, "/"))
	if clean == "." || strings.HasPrefix(clean, "..") {
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
	p, e := l.path(key)
	if e != nil {
		return Object{}, e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return Object{}, e
	}
	tmp := p + ".partial"
	f, e := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return Object{}, e
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, l.Max+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n != size || n > l.Max {
		os.Remove(tmp)
		return Object{}, errors.New("object size mismatch")
	}
	if e = os.Rename(tmp, p); e != nil {
		os.Remove(tmp)
		return Object{}, e
	}
	return Object{key, mime, hex.EncodeToString(h.Sum(nil)), n}, nil
}
func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, e := l.path(key)
	if e != nil {
		return nil, e
	}
	return os.Open(p)
}
func (l *Local) Delete(_ context.Context, key string) error {
	p, e := l.path(key)
	if e != nil {
		return e
	}
	e = os.Remove(p)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
