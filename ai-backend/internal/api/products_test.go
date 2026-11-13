package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qa-automation/ai-backend/internal/auth"
	"github.com/qa-automation/ai-backend/internal/models"
)

func TestValidateProduct(t *testing.T) {
	ok := models.Product{Name: " HISv3 ", RepoURL: "https://github.com/org/repo", StagingURL: "https://staging.example.com", RootDir: "/org/repo/", Branch: "main"}
	if msg := validateProduct(&ok); msg != "" {
		t.Fatalf("valid product rejected: %s", msg)
	}
	if ok.Name != "HISv3" || ok.RootDir != "org/repo" {
		t.Fatalf("fields must be trimmed/normalised: %+v", ok)
	}
	bad := []models.Product{
		{Name: "", RepoURL: ok.RepoURL, StagingURL: ok.StagingURL, RootDir: "a"},
		{Name: "x", RepoURL: "https://evil.com/o/r", StagingURL: ok.StagingURL, RootDir: "a"},
		{Name: "x", RepoURL: ok.RepoURL, StagingURL: "javascript:alert(1)", RootDir: "a"},
		{Name: "x", RepoURL: ok.RepoURL, StagingURL: "ftp://h/x", RootDir: "a"},
		{Name: "x", RepoURL: ok.RepoURL, StagingURL: ok.StagingURL, RootDir: "../etc"},
		{Name: "x", RepoURL: ok.RepoURL, StagingURL: ok.StagingURL, RootDir: "a", Branch: "--upload-pack=x"},
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
		"Service": {false, false}, // rights only via impersonation
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

func TestRootDirOverlap(t *testing.T) {
	others := []string{"org/repo", "solo"}
	for _, rel := range []string{"org/repo", "org/repo/sub", "org", "solo"} {
		if !rootDirOverlap(rel, others) {
			t.Errorf("%q should overlap", rel)
		}
	}
	for _, rel := range []string{"org/repo2", "other", "solo2", "orgx"} {
		if rootDirOverlap(rel, others) {
			t.Errorf("%q should not overlap", rel)
		}
	}
}

func TestValidateProduct_RejectsStagingCredentials(t *testing.T) {
	p := models.Product{Name: "x", RepoURL: "https://github.com/o/r", StagingURL: "https://user:pw@staging.example.com", RootDir: "a"}
	if msg := validateProduct(&p); msg == "" {
		t.Fatal("credentials in staging_url must be rejected")
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

func TestProductVerificationStale(t *testing.T) {
	base := models.Product{RepoURL: "https://github.com/o/r", StagingURL: "https://s", RootDir: "o/r", Name: "a", Branch: "main"}
	same := base
	same.Name, same.Branch = "b", "develop" // name/branch edits keep the verification
	if productVerificationStale(base, same) {
		t.Fatal("name/branch change must not invalidate verification")
	}
	for _, mut := range []func(*models.Product){
		func(p *models.Product) { p.RepoURL = "https://github.com/o/other" },
		func(p *models.Product) { p.StagingURL = "https://t" },
		func(p *models.Product) { p.RootDir = "x/y" },
	} {
		n := base
		mut(&n)
		if !productVerificationStale(base, n) {
			t.Fatalf("change must invalidate verification: %+v", n)
		}
	}
}
