package objectstore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/qa-automation/ai-backend/internal/config"
)

func noSuchKey() error {
	return minio.ErrorResponse{Code: "NoSuchKey", Message: "secret-host bucket key"}
}

func TestValidateKey(t *testing.T) {
	ok := []string{"qa-automation/a", "qa-automation/prod/knowledge-exports/1/x.pdf", "qa-automation/a.b/c-d_e"}
	for _, k := range ok {
		if err := validateKey(Prefix, k); err != nil {
			t.Errorf("%q: %v", k, err)
		}
	}
	bad := []string{
		"", "other/x", "/qa-automation/x", "qa-automation/", "qa-automation", "qa-automation//x", "qa-automation/a//b",
		"qa-automation/./x", "qa-automation/a/../b", "qa-automation/..", "qa-automation/a\\b", "qa-automation/a\x00b",
		"qa-automation/a\nb", "qa-automation/a\x7fb", "qa-automation/a/", "QA-AUTOMATION/x", "../qa-automation/x",
	}
	for _, k := range bad {
		if err := validateKey(Prefix, k); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%q must be refused, got %v", k, err)
		}
	}
}

func TestConstructorRejectsBadPrefix(t *testing.T) {
	for _, p := range []string{"", "qa-automation", "a"} {
		if _, err := newStore(newFakeAPI(), "b", p, time.Second); err == nil {
			t.Errorf("prefix %q accepted", p)
		}
	}
	if _, err := newStore(newFakeAPI(), "", Prefix, time.Second); err == nil {
		t.Error("empty bucket accepted")
	}
}

func TestNewRefusesBadConfig(t *testing.T) {
	cfg := config.ObjectStoreConfig{Endpoint: "10.8.0.5:9000", AccessKey: "ak-SECRET", SecretKey: "sk-SECRET", Bucket: "b", KeyNamespace: "Bad_NS", UseSSL: false}
	s, err := New(cfg)
	if s != nil || err == nil {
		t.Fatal("invalid namespace accepted")
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "10.8.0.5") {
		t.Errorf("error leaks a value: %v", err)
	}
	cfg.KeyNamespace = "dev-x"
	if s, err := New(cfg); s == nil || err != nil {
		t.Fatalf("valid config refused: %v", err) // lazy: nothing is contacted
	}
}

func TestNoListOrDelete(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	if strings.Join(names, ",") != "Get,Put,Stat" {
		t.Fatalf("Store methods = %v", names)
	}
	m := reflect.TypeOf(&MinioStore{})
	for _, n := range []string{"List", "Delete", "Remove"} {
		if _, ok := m.MethodByName(n); ok {
			t.Errorf("MinioStore has %s", n)
		}
	}
}

func TestInvalidKeyMakesNoCall(t *testing.T) {
	f := newFakeAPI()
	s, _ := newStore(f, "b", Prefix, time.Second)
	ctx := context.Background()
	if _, _, err := s.Stat(ctx, "sut/x"); !errors.Is(err, ErrInvalidKey) {
		t.Error(err)
	}
	if _, _, err := s.Get(ctx, "qa-automation/../x"); !errors.Is(err, ErrInvalidKey) {
		t.Error(err)
	}
	if err := s.Put(ctx, "qa-automation//x", "text/plain", nil); !errors.Is(err, ErrInvalidKey) {
		t.Error(err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("network called for invalid keys: %v", f.calls)
	}
}

func TestStoreRoundTripAndNotFound(t *testing.T) {
	f := newFakeAPI()
	s, _ := newStore(f, "b", Prefix, time.Second)
	ctx := context.Background()
	const k = "qa-automation/ns/x.csv"
	if b, found, err := s.Get(ctx, k); b != nil || found || err != nil {
		t.Fatalf("missing Get: %v %v %v", b, found, err)
	}
	if _, found, err := s.Stat(ctx, k); found || err != nil {
		t.Fatalf("missing Stat: %v %v", found, err)
	}
	if err := s.Put(ctx, k, "text/csv", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if b, found, err := s.Get(ctx, k); string(b) != "hi" || !found || err != nil {
		t.Fatalf("Get: %q %v %v", b, found, err)
	}
	if in, found, err := s.Stat(ctx, k); !found || err != nil || in.Size != 2 {
		t.Fatalf("Stat: %+v %v %v", in, found, err)
	}
}

func TestErrorsCarryCodeOnly(t *testing.T) {
	f := newFakeAPI()
	f.err = minio.ErrorResponse{Code: "AccessDenied", Message: "host=10.1.1.1 bucket=sut-prod key=qa-automation/x request=abc"}
	s, _ := newStore(f, "b", Prefix, time.Second)
	ctx := context.Background()
	_, _, e1 := s.Get(ctx, "qa-automation/x")
	_, _, e2 := s.Stat(ctx, "qa-automation/x")
	e3 := s.Put(ctx, "qa-automation/x", "a/b", []byte("x"))
	for i, e := range []error{e1, e2, e3} {
		if e == nil || ErrCode(e) != "AccessDenied" || strings.Contains(e.Error(), "10.1.1.1") || strings.Contains(e.Error(), "sut") {
			t.Errorf("error %d = %v", i, e)
		}
	}
	if ErrOp(e1) != "get" || ErrOp(e2) != "stat" || ErrOp(e3) != "put" {
		t.Errorf("ops: %s %s %s", ErrOp(e1), ErrOp(e2), ErrOp(e3))
	}
	f.err = errors.New("dial tcp 10.1.1.1:9000: refused")
	_, _, e := s.Get(ctx, "qa-automation/x")
	if strings.Contains(e.Error(), "10.1.1.1") || ErrCode(e) != "*errors.errorString" {
		t.Errorf("non-S3 error: %v", e)
	}
}

func TestGetTooLarge(t *testing.T) {
	f := newFakeAPI()
	s, _ := newStore(f, "b", Prefix, time.Second)
	f.err = errTooLarge
	if _, _, err := s.Get(context.Background(), "qa-automation/x"); ErrCode(err) != "TooLarge" {
		t.Errorf("err = %v", err)
	}
}
