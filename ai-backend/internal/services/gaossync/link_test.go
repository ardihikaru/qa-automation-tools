package gaossync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func linkSyncer(t *testing.T, h http.HandlerFunc) *Syncer {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s := &Syncer{configured: true, db: nil}
	s.cl = &client{base: srv.URL + "/api/v1", token: "gaos_x", hc: newHTTPClient(), timeout: 2 * time.Second, sleep: sleepCtx}
	return s
}

func TestCheckLink(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"linked", 200, `{"products":[{"id":"p-6","name":"HISv3"}]}`, LinkLinked},
		{"not scoped", 200, `{"products":[{"id":"p-1"}]}`, LinkNotScoped},
		{"unauthorized", 401, ``, LinkUnauthorized},
		{"server error", 500, ``, LinkUnreachable},
		{"bad body", 200, `nope`, LinkUnreachable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := linkSyncer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/products" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer gaos_x" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			})
			// Configured() needs db != nil; call the check path directly through get.
			got, _ := checkLinkWith(context.Background(), s, "p-6")
			if got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}

func TestWebURL(t *testing.T) {
	s := &Syncer{cl: &client{base: "https://gaos.sismedika.com/api/v1"}}
	if got := s.WebURL(); got != "https://gaos.sismedika.com/app/" {
		t.Fatalf("got %q", got)
	}
	s = &Syncer{cl: &client{base: "http://10.10.0.9:18301/api/v1"}}
	if s.WebURL() != "" {
		t.Fatal("an http API URL must not be exposed as the web link")
	}
	s.SetWebURL("https://gaos.sismedika.com/app/")
	if s.WebURL() != "https://gaos.sismedika.com/app/" {
		t.Fatalf("GAOS_WEB_URL not used: %q", s.WebURL())
	}
	s.SetWebURL("javascript:alert(1)")
	if s.WebURL() != "https://gaos.sismedika.com/app/" {
		t.Fatal("an invalid GAOS_WEB_URL must be ignored")
	}
	if (*Syncer)(nil).WebURL() != "" {
		t.Fatal("nil syncer must give an empty URL")
	}
}
