package knowledge

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"time"
)

// Spec: docs/designs/MODULE_CHECKLIST_EXPORT.md, Amendment 3 (evidence nodes, last update).

// EvidenceItem is one piece of evidence of a feature row, parsed from its Evidence cell.
type EvidenceItem struct {
	Kind  string `json:"kind"`           // GIT, SRV, DOC, RN, S, BRD or TEXT
	Raw   string `json:"raw"`            // the tag as written
	Label string `json:"label"`          // what it is, for people
	Repo  string `json:"repo,omitempty"` // GIT: product repository
	SHA   string `json:"sha,omitempty"`  // GIT: merge commit on main
	Date  string `json:"date,omitempty"` // YYYY-MM-DD when the tag carries one
	URL   string `json:"url,omitempty"`  // GIT with repo and sha: the commit page
}

var (
	evidenceTagRe = regexp.MustCompile(`\b(GIT|SRV|DOC|RN|S|BRD):[^\s;·,|]+`)
	gitCommitRe   = regexp.MustCompile(`^([A-Za-z0-9._-]+)@([0-9a-f]{7,40})(?:/(\d{4}-\d{2}-\d{2}))?$`)
	gitDateRe     = regexp.MustCompile(`^main@(\d{4}-\d{2}-\d{2})$`)
	yyyymmddRe    = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})$`)
	repoOwnerRe   = regexp.MustCompile(`github\.com[:/]([A-Za-z0-9._-]+)/`)
)

// ParseEvidence splits an Evidence cell into items. owner is the GitHub owner of the product's
// repositories ("" = no links). Text without any known tag becomes one TEXT item.
func ParseEvidence(cell, owner string) []EvidenceItem {
	cell = strings.TrimSpace(cell)
	if cell == "" || cell == "—" {
		return []EvidenceItem{}
	}
	tags := evidenceTagRe.FindAllString(cell, -1)
	if len(tags) == 0 {
		return []EvidenceItem{{Kind: "TEXT", Raw: cell, Label: cell}}
	}
	out := make([]EvidenceItem, 0, len(tags))
	for _, t := range tags {
		kind, val, _ := strings.Cut(t, ":")
		it := EvidenceItem{Kind: kind, Raw: t}
		switch kind {
		case "GIT":
			switch {
			case val == "main":
				it.Label = "Code on main"
			case val == "stg" || val == "staging":
				it.Label = "Code on staging only"
			case gitDateRe.MatchString(val):
				it.Label = "Code reached main"
				it.Date = gitDateRe.FindStringSubmatch(val)[1]
			case gitCommitRe.MatchString(val):
				m := gitCommitRe.FindStringSubmatch(val)
				it.Repo, it.SHA, it.Date = m[1], m[2], m[3]
				it.Label = "Merged into main: " + it.Repo + " " + it.SHA
				if owner != "" {
					it.URL = "https://github.com/" + owner + "/" + it.Repo + "/commit/" + it.SHA
				}
			default:
				it.Label = "Code: " + val
			}
		case "SRV":
			it.Label = "Release survey"
			it.Date = isoFromYYYYMMDD(val)
		case "RN":
			it.Label = "Release note " + val
			it.Date = isoFromYYYYMMDD(val)
		case "DOC":
			it.Label = "Document: " + val
		case "S":
			it.Label = "Plan sheet row " + val
		case "BRD":
			it.Label = "BRD domain " + val
		}
		out = append(out, it)
	}
	return out
}

func isoFromYYYYMMDD(s string) string {
	if m := yyyymmddRe.FindStringSubmatch(s); m != nil {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	return ""
}

// repoOwner returns the GitHub owner of the product's automation repository (else any of its
// repositories), or "" when none is on GitHub.
func (s *Syncer) repoOwner(ctx context.Context, pid int) string {
	var url string
	err := s.DB.QueryRow(ctx, `SELECT repo_url FROM product_repos WHERE product_id = ? ORDER BY is_automation DESC, id LIMIT 1`, pid).Scan(&url)
	if err != nil {
		return ""
	}
	if m := repoOwnerRe.FindStringSubmatch(url); m != nil {
		return m[1]
	}
	return ""
}

// syncRef describes one sync for an entity page: when it finished (RFC 3339, UTC) and on which
// catalogue commit. nil when the id is absent or unknown.
func (s *Syncer) syncRef(ctx context.Context, pid int, id any) Item {
	n, ok := id.(int64)
	if !ok || n == 0 {
		return nil
	}
	var started, finished sql.NullTime
	var sha sql.NullString
	if err := s.DB.QueryRow(ctx, `SELECT started_at, finished_at, commit_sha FROM kb_syncs WHERE product_id = ? AND id = ?`, pid, n).Scan(&started, &finished, &sha); err != nil {
		return nil
	}
	it := Item{"id": n, "commit_sha": sha.String}
	switch {
	case finished.Valid:
		it["finished_at"] = finished.Time.UTC().Format(time.RFC3339)
	case started.Valid:
		it["finished_at"] = started.Time.UTC().Format(time.RFC3339)
	}
	return it
}
