package gaossync

// GAOS_RELEASE_B.md §4 step 2: right before the QA tool turns on the pinned auto-merge of an
// approved spec, it asks GAOS whether the product still allows it. GAOS answers by its own rule
// (the product's switch and readiness band) at that moment.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// SpecAutoMergeAllowed asks GAOS GET /products/{id}/spec-auto-merge. Any failure is an error,
// never an "allowed": the caller refuses the auto-merge.
func (s *Syncer) SpecAutoMergeAllowed(ctx context.Context, gaosPID string) (bool, error) {
	if !s.Configured() {
		return false, &Failure{Note: "spec auto-merge: GAOS is not configured"}
	}
	if !ValidProductID(gaosPID) {
		return false, &Failure{Note: "spec auto-merge: invalid GAOS product id"}
	}
	status, data, f := s.cl.get(ctx, "/products/"+url.PathEscape(gaosPID)+"/spec-auto-merge")
	if f != nil {
		f.Note = "spec auto-merge: " + f.Note
		return false, f
	}
	if status != http.StatusOK {
		return false, &Failure{Status: status, Note: "spec auto-merge: GAOS answered HTTP " + http.StatusText(status)}
	}
	var a struct {
		Allowed *bool `json:"allowed"`
	}
	if err := json.Unmarshal(data, &a); err != nil || a.Allowed == nil {
		return false, &Failure{Note: "spec auto-merge: GAOS answered with an unexpected body"}
	}
	return *a.Allowed, nil
}
