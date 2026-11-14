package gaossync

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Link states reported by CheckLink.
const (
	LinkLinked       = "linked"       // GAOS answered and lists the product for this token
	LinkNotScoped    = "not_scoped"   // GAOS answered, but the product is not enabled for this token
	LinkUnauthorized = "unauthorized" // 401 / 403: the token was refused
	LinkUnreachable  = "unreachable"  // transport failure, timeout or a bad answer
)

// SetWebURL sets the public GAOS web app URL the dashboard links to (GAOS_WEB_URL). An invalid
// value (not http/https, has userinfo) is ignored and logged by name only.
func (s *Syncer) SetWebURL(raw string) {
	raw = strings.TrimSpace(raw)
	if s == nil || raw == "" {
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		log.Printf("gaossync: GAOS_WEB_URL is invalid; ignored")
		return
	}
	s.webURL = u.String()
}

// WebURL is the URL the products badge links to: GAOS_WEB_URL when set, else the https origin of
// GAOS_API_URL plus /app/ (an http API URL is usually an internal address, so it is not exposed).
// "" when nothing usable is known.
func (s *Syncer) WebURL() string {
	if s == nil {
		return ""
	}
	if s.webURL != "" {
		return s.webURL
	}
	if s.cl == nil {
		return ""
	}
	u, err := url.Parse(s.cl.base)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/app/"
}

// CheckLink asks GAOS (GET /products, read-only) whether productID is enabled for the QA token.
// The returned note never holds a token, a URL or a response body.
func (s *Syncer) CheckLink(ctx context.Context, productID string) (state, note string) {
	if !s.Configured() {
		return LinkUnreachable, "GAOS is not configured"
	}
	return checkLinkWith(ctx, s, productID)
}

func checkLinkWith(ctx context.Context, s *Syncer, productID string) (state, note string) {
	status, data, f := s.cl.get(ctx, "/products")
	if f != nil {
		return LinkUnreachable, f.Note
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return LinkUnauthorized, hints[status]
	default:
		return LinkUnreachable, "GAOS answered an unexpected status"
	}
	var body struct {
		Products []struct {
			ID string `json:"id"`
		} `json:"products"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return LinkUnreachable, "GAOS answered 200 with an unreadable body"
	}
	for _, p := range body.Products {
		if strings.EqualFold(p.ID, productID) {
			return LinkLinked, ""
		}
	}
	return LinkNotScoped, "the GAOS product id is not enabled for this token"
}
