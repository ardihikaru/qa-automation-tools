package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/executor"
)

func TestHealthReportsExecutor(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer fake.Close()
	router := NewRouter(nil, nil, nil, nil)
	router.SetExecutor(executor.New(executor.Config{URL: fake.URL, Token: "t"}), time.Minute)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if !strings.Contains(rec.Body.String(), `"executor":"ok"`) {
		t.Fatalf("health missing executor: %s", rec.Body.String())
	}
}
