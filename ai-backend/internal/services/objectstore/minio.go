package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qa-automation/ai-backend/internal/config"
)

// api is the part of the minio client this package uses; tests replace it.
type api interface {
	stat(ctx context.Context, bucket, key string) (Info, error)
	get(ctx context.Context, bucket, key string, max int64) ([]byte, error)
	put(ctx context.Context, bucket, key, contentType string, data []byte) error
}

// MinioStore implements Store on MinIO / S3. The client is unexported.
type MinioStore struct {
	c       api
	bucket  string
	prefix  string
	timeout time.Duration
}

// New validates cfg and builds the store. It returns a nil store and an error when the config is
// unset or invalid (the error names settings, never values). The connection is lazy: nothing is
// contacted here, so an outage at startup does not stop the server.
func New(cfg config.ObjectStoreConfig) (*MinioStore, error) {
	if err := cfg.Check(); err != nil {
		return nil, err
	}
	cl, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, &OpError{Op: "init", Code: fmt.Sprintf("%T", err)}
	}
	return newStore(&minioAPI{cl: cl}, cfg.Bucket, Prefix, cfg.Timeout())
}

func newStore(c api, bucket, prefix string, timeout time.Duration) (*MinioStore, error) {
	if prefix == "" || prefix[len(prefix)-1] != '/' {
		return nil, fmt.Errorf("objectstore: prefix must be non-empty and end in '/'")
	}
	if bucket == "" {
		return nil, fmt.Errorf("objectstore: bucket is empty")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &MinioStore{c: c, bucket: bucket, prefix: prefix, timeout: timeout}, nil
}

func opErr(op string, err error) error {
	if code := minio.ToErrorResponse(err).Code; code != "" {
		return &OpError{Op: op, Code: code}
	}
	return &OpError{Op: op, Code: fmt.Sprintf("%T", err)}
}

func isNotFound(err error) bool {
	switch minio.ToErrorResponse(err).Code {
	case "NoSuchKey", "NotFound":
		return true
	}
	return false
}

// Stat implements Store.
func (s *MinioStore) Stat(ctx context.Context, key string) (Info, bool, error) {
	if err := validateKey(s.prefix, key); err != nil {
		return Info{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	info, err := s.c.stat(ctx, s.bucket, key)
	switch {
	case err == nil:
		return info, true, nil
	case isNotFound(err):
		return Info{}, false, nil
	}
	return Info{}, false, opErr("stat", err)
}

// Get implements Store. An object larger than MaxGetBytes is an error.
func (s *MinioStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if err := validateKey(s.prefix, key); err != nil {
		return nil, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	b, err := s.c.get(ctx, s.bucket, key, MaxGetBytes)
	switch {
	case err == nil:
		return b, true, nil
	case isNotFound(err):
		return nil, false, nil
	case err == errTooLarge:
		return nil, false, &OpError{Op: "get", Code: "TooLarge"}
	}
	return nil, false, opErr("get", err)
}

// Put implements Store.
func (s *MinioStore) Put(ctx context.Context, key, contentType string, data []byte) error {
	if err := validateKey(s.prefix, key); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if err := s.c.put(ctx, s.bucket, key, contentType, data); err != nil {
		return opErr("put", err)
	}
	return nil
}

var errTooLarge = fmt.Errorf("object too large")

type minioAPI struct{ cl *minio.Client }

func (m *minioAPI) stat(ctx context.Context, bucket, key string) (Info, error) {
	st, err := m.cl.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return Info{}, err
	}
	return Info{Size: st.Size, LastModified: st.LastModified}, nil
}

func (m *minioAPI) get(ctx context.Context, bucket, key string, max int64) ([]byte, error) {
	o, err := m.cl.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer o.Close()
	b, err := io.ReadAll(io.LimitReader(o, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errTooLarge
	}
	return b, nil
}

func (m *minioAPI) put(ctx context.Context, bucket, key, contentType string, data []byte) error {
	_, err := m.cl.PutObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	return err
}
