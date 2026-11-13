package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/models"
)

func TestValidateProduct(t *testing.T) {
	ok := models.Product{Name: " HISv3 ", StagingURL: " https://staging.example.com "}
	if msg := validateProduct(&ok); msg != "" {
		t.Fatalf("valid product rejected: %s", msg)
	}
	if ok.Name != "HISv3" || ok.StagingURL != "https://staging.example.com" {
		t.Fatalf("fields must be trimmed: %+v", ok)
	}
	bad := []models.Product{
		{Name: "", StagingURL: ok.StagingURL},
		{Name: strings.Repeat("n", 101), StagingURL: ok.StagingURL},
		{Name: "x", StagingURL: "javascript:alert(1)"},
		{Name: "x", StagingURL: "ftp://h/x"},
		{Name: "x", StagingURL: "https://user:pw@staging.example.com"},
	}
	for i, p := range bad {
		p := p
		if msg := validateProduct(&p); msg == "" {
			t.Errorf("case %d should be rejected: %+v", i, p)
		}
	}
}

func TestProductRole(t *testing.T) {
	cases := map[string][2]bool{
		"Admin": {true, true}, "DevOps": {true, true}, "Developer": {true, true},
		"QA": {false, true}, "QC": {false, true}, "UI/UX Designer": {false, false},
	}
	for role, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
		req = req.WithContext(auth.SetUser(req.Context(), &auth.User{Role: role}))
		m, v := productRole(req)
		if m != want[0] || v != want[1] {
			t.Errorf("%s: got manage=%v verify=%v, want %v", role, m, v, want)
		}
	}
}

func TestProductRole_FailsClosedWithoutUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	if m, v := productRole(req); m || v {
		t.Fatalf("no user must mean no access, got manage=%v verify=%v", m, v)
	}
}

func TestHandleProducts_NoDB(t *testing.T) {
	r := &Router{}
	rec := httptest.NewRecorder()
	r.handleProducts(rec, httptest.NewRequest(http.MethodGet, "/api/products", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}
