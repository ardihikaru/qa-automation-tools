package api

// GAOS_RELEASE_B.md §3.1: GET /api/v1/gaos/capabilities tells GAOS which optional request fields
// this build understands, so GAOS never sends one an older build would reject or misread.

import (
	"net/http"
	"os"
	"strings"
)

const gaosCapabilitiesPath = "/api/v1/gaos/capabilities"

// specAutoMergeOn is the QA_SPEC_AUTO_MERGE switch (default off).
func specAutoMergeOn() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("QA_SPEC_AUTO_MERGE")), "on")
}

// gaosCapabilities answers the capabilities of this build: "kind" always, "auto-merge" only when
// the operator turned the switch on.
func (r *Router) gaosCapabilities(w http.ResponseWriter, _ *http.Request) {
	caps := []string{"kind"}
	if specAutoMergeOn() {
		caps = append(caps, "auto-merge")
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": caps})
}
