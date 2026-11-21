package gaossync

// GAOS Inbox client (CASE_AUTOMATION.md section 6): the QA tools ask a human a question in GAOS
// (here: "approve this generated automation PR?") and read the answer back.
//
//	POST {base}/products/{pid}/inbox                 create (201 new, 200 already there)
//	GET  {base}/products/{pid}/inbox/{externalId}    read the item
//	POST {base}/products/{pid}/inbox/{externalId}/answer  answer it (HISTORY_ANSWER.md)
//
// Same conventions as the push: the QA bearer token, no redirects, 429 retried, notes without
// URLs or bodies. A non-2xx or an unknown status is the caller's "still open".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

// Inbox statuses GAOS reports.
const (
	InboxOpen     = "open"
	InboxAnswered = "answered"
	InboxExpired  = "expired"
)

var inboxExternalIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

// InboxOption is one answer button.
type InboxOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// InboxCreate is the body of POST .../inbox.
type InboxCreate struct {
	ExternalID     string        `json:"externalId"`
	Title          string        `json:"title"`
	Question       string        `json:"question"`
	Context        string        `json:"context"`
	Options        []InboxOption `json:"options"`
	ExpiresInHours int           `json:"expiresInHours"`
}

// InboxCreated is GAOS's answer to a create.
type InboxCreated struct {
	ID         json.RawMessage `json:"id"`
	ExternalID string          `json:"externalId"`
	Status     string          `json:"status"`
	Created    bool            `json:"created"`
}

// InboxItem is the answer to a read.
type InboxItem struct {
	ExternalID  string          `json:"externalId"`
	Status      string          `json:"status"` // open | answered | expired
	Choice      json.RawMessage `json:"choice"` // an index or a key; kept raw
	OptionLabel string          `json:"optionLabel"`
	Text        string          `json:"text"`
	AnsweredAt  json.RawMessage `json:"answeredAt"`
	// Source is who answered and through which client, as GAOS reports it (HISTORY_ANSWER.md);
	// kept raw, GAOS owns its shape ({kind, client, origin, actor}).
	Source json.RawMessage `json:"source,omitempty"`
}

// InboxActor is who answers, sent to GAOS with an answer.
type InboxActor struct {
	Username string `json:"username"`
	Display  string `json:"display,omitempty"`
}

// InboxAnswer is the body of POST .../inbox/{externalId}/answer. Choice is the 1-based option
// index in the order the item was created with.
type InboxAnswer struct {
	Choice int        `json:"choice"`
	Text   string     `json:"text,omitempty"`
	Actor  InboxActor `json:"actor"`
}

// InboxAnswerResult is GAOS's 200 (Already false) or 409 already_answered (Already true): the
// item's current answer either way.
type InboxAnswerResult struct {
	Item    InboxItem
	Already bool
}

func (s *Syncer) inboxReady(gaosPID, externalID string) error {
	if s == nil || !s.configured || s.cl == nil {
		return ErrNotConfigured
	}
	if !ValidProductID(gaosPID) {
		return errors.New("bad GAOS product id")
	}
	if !inboxExternalIDRe.MatchString(externalID) {
		return errors.New("bad externalId")
	}
	return nil
}

// CreateInboxItem creates the item (idempotent on externalId: GAOS answers 200 when it exists).
func (s *Syncer) CreateInboxItem(ctx context.Context, gaosPID string, req InboxCreate) (*InboxCreated, error) {
	if err := s.inboxReady(gaosPID, req.ExternalID); err != nil {
		return nil, err
	}
	if len(req.Options) == 0 {
		return nil, errors.New("an inbox item needs options")
	}
	var out InboxCreated
	if f := s.cl.postOK(ctx, "/products/"+gaosPID+"/inbox", "inbox create", req, &out, http.StatusCreated); f != nil {
		return nil, f
	}
	if out.ExternalID != req.ExternalID || out.Status == "" {
		return nil, &Failure{Note: "inbox create: GAOS answered with an unexpected body"}
	}
	return &out, nil
}

// GetInboxItem reads the item. Any non-200 answer is an error (the caller keeps polling).
func (s *Syncer) GetInboxItem(ctx context.Context, gaosPID, externalID string) (*InboxItem, error) {
	if err := s.inboxReady(gaosPID, externalID); err != nil {
		return nil, err
	}
	status, data, f := s.cl.get(ctx, "/products/"+url.PathEscape(gaosPID)+"/inbox/"+url.PathEscape(externalID))
	if f != nil {
		f.Note = "inbox read: " + f.Note
		return nil, f
	}
	if status != http.StatusOK {
		return nil, &Failure{Status: status, Note: "inbox read: GAOS answered HTTP " + http.StatusText(status)}
	}
	var it InboxItem
	if err := json.Unmarshal(data, &it); err != nil || it.ExternalID != externalID {
		return nil, &Failure{Note: "inbox read: GAOS answered with an unexpected body"}
	}
	return &it, nil
}

// AnswerInboxItem answers the item in GAOS (HISTORY_ANSWER.md). One request, no retry: a 429
// is a Failure like any other non-200/409 answer, so the caller can fall back at once. The
// caller sets the time budget through ctx (the client's own limit is requestTimeout).
func (s *Syncer) AnswerInboxItem(ctx context.Context, gaosPID, externalID string, a InboxAnswer) (*InboxAnswerResult, error) {
	if err := s.inboxReady(gaosPID, externalID); err != nil {
		return nil, err
	}
	if a.Choice < 1 || a.Actor.Username == "" {
		return nil, errors.New("an answer needs a choice and an actor")
	}
	payload, err := json.Marshal(a)
	if err != nil {
		return nil, &Failure{Note: "inbox answer: could not encode the answer"}
	}
	status, _, data, f := s.cl.once(ctx, "/products/"+url.PathEscape(gaosPID)+"/inbox/"+url.PathEscape(externalID)+"/answer", payload)
	if f != nil {
		f.Note = "inbox answer: " + f.Note
		return nil, f
	}
	switch status {
	case http.StatusOK, http.StatusConflict:
		var it InboxItem
		if err := json.Unmarshal(data, &it); err != nil || it.ExternalID != externalID {
			return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered with an unexpected body"}
		}
		return &InboxAnswerResult{Item: it, Already: status == http.StatusConflict}, nil
	case http.StatusNotFound:
		return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered HTTP 404 (GAOS does not know this item, or has no answer endpoint yet)"}
	case http.StatusGone:
		return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered HTTP 410 (the item expired in GAOS)"}
	case http.StatusUnprocessableEntity:
		return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered HTTP 422 (GAOS refused the choice)"}
	}
	note := fmt.Sprintf("inbox answer: GAOS answered HTTP %d", status)
	if h, ok := hints[status]; ok && status != http.StatusNotFound {
		note += " (" + h + ")"
	}
	return nil, &Failure{Status: status, Note: note}
}
