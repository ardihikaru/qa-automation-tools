package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func captureWrites(t *testing.T) *[]string {
	t.Helper()
	var lines []string
	orig := logImpersonatedWrite
	logImpersonatedWrite = func(u *User, method, path string, status int) {
		lines = append(lines, u.Impersonator.Username+">"+u.Username+" "+method+" "+path+" "+http.StatusText(status))
	}
	t.Cleanup(func() { logImpersonatedWrite = orig })
	return &lines
}

func TestServeAs_LogsWritesOnlyWhileImpersonating(t *testing.T) {
	lines := captureWrites(t)
	imp := &User{Username: "qa1", Role: "QA", Impersonator: &Actor{Username: "ai-service", Role: "Service"}}
	real := &User{Username: "admin", Role: "Admin"}

	created := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := GetUser(r.Context()); !ok || u.Username == "" {
			t.Error("user must be on the context")
		}
		w.WriteHeader(http.StatusCreated)
	})
	implicit200 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })

	cases := []struct {
		u      *User
		method string
		h      http.Handler
	}{
		{imp, http.MethodPost, created},       // logged, 201
		{imp, http.MethodDelete, implicit200}, // logged, defaults to 200
		{imp, http.MethodGet, created},        // not logged
		{real, http.MethodPost, created},      // not logged
	}
	for _, c := range cases {
		serveAs(httptest.NewRecorder(), httptest.NewRequest(c.method, "/api/x", nil), c.u, c.h)
	}

	want := []string{"ai-service>qa1 POST /api/x Created", "ai-service>qa1 DELETE /api/x OK"}
	if len(*lines) != len(want) {
		t.Fatalf("got %d lines %v, want %v", len(*lines), *lines, want)
	}
	for i := range want {
		if (*lines)[i] != want[i] {
			t.Errorf("line %d: got %q want %q", i, (*lines)[i], want[i])
		}
	}
}

func TestStatusWriter_FirstWriteHeaderWins(t *testing.T) {
	sw := &statusWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	sw.WriteHeader(http.StatusForbidden)
	sw.WriteHeader(http.StatusOK)
	if sw.status != http.StatusForbidden {
		t.Fatalf("got %d", sw.status)
	}
}
