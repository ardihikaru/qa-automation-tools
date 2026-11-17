// Package objectstore is a minimal, prefix-locked S3 client for the export cache
// (MODULE_CHECKLIST_EXPORT.md). The bucket is shared with SUT, so the package offers only
// Stat, Get and Put, and refuses every key outside its fixed prefix. There is deliberately no
// List and no Delete.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Prefix is the fixed key prefix of everything this app writes. Callers build keys under it.
const Prefix = "qa-automation/"

// MaxGetBytes caps what Get reads (export objects are small).
const MaxGetBytes = 64 << 20

// Info describes a stored object.
type Info struct {
	Size         int64
	LastModified time.Time
}

// Store is the whole surface the app may use. No List, no Delete.
type Store interface {
	// Stat reports whether the object exists; Info is valid only then.
	Stat(ctx context.Context, key string) (Info, bool, error)
	// Get returns the object bytes; a missing object is (nil, false, nil).
	Get(ctx context.Context, key string) ([]byte, bool, error)
	// Put stores data under key, overwriting any existing object.
	Put(ctx context.Context, key, contentType string, data []byte) error
}

// ErrInvalidKey means a key was refused before any network call.
var ErrInvalidKey = errors.New("objectstore: invalid key")

// OpError is a store failure reduced to the operation and an error code. It never carries the
// endpoint, bucket, key or the remote message (they could identify shared infrastructure).
type OpError struct {
	Op   string // stat, get or put
	Code string // S3 error code, or the Go type of a non-S3 error
}

func (e *OpError) Error() string { return fmt.Sprintf("objectstore %s: %s", e.Op, e.Code) }

// ErrCode returns the code of an OpError ("unknown" for anything else), for log lines.
func ErrCode(err error) string {
	var oe *OpError
	if errors.As(err, &oe) {
		return oe.Code
	}
	return "unknown"
}

// ErrOp returns the operation of an OpError ("unknown" for anything else).
func ErrOp(err error) string {
	var oe *OpError
	if errors.As(err, &oe) {
		return oe.Op
	}
	return "unknown"
}

// validateKey enforces the prefix and path-safety rules. It is called by every Store method
// before any network call.
func validateKey(prefix, key string) error {
	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("%w: bad prefix", ErrInvalidKey)
	}
	if !strings.HasPrefix(key, prefix) {
		return fmt.Errorf("%w: outside the prefix", ErrInvalidKey)
	}
	rest := key[len(prefix):]
	if rest == "" {
		return fmt.Errorf("%w: empty remainder", ErrInvalidKey)
	}
	if strings.HasPrefix(key, "/") || strings.ContainsRune(key, '\\') {
		return fmt.Errorf("%w: bad character", ErrInvalidKey)
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("%w: control character", ErrInvalidKey)
		}
	}
	for _, seg := range strings.Split(rest, "/") {
		switch seg {
		case "":
			return fmt.Errorf("%w: empty segment", ErrInvalidKey)
		case ".", "..":
			return fmt.Errorf("%w: dot segment", ErrInvalidKey)
		}
	}
	return nil
}
