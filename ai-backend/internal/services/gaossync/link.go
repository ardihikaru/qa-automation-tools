package gaossync

import (
	"context"
	"encoding/json"
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

// WebURL is the GAOS web app (back office) derived from GAOS_API_URL: the origin plus /app/.
// "" when the service is not configured.
func (s *Syncer) WebURL() string {
	if s == nil || s.cl == nil {
		return ""
	}
	u, err := url.Parse(s.cl.base)
	if err != nil || u.Host == "" {
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
