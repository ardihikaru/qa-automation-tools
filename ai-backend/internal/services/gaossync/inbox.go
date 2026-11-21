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
	"time"
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

// InboxAnswerResult is GAOS's 200 (ours), 409 already_answered (Already: someone answered
// first) or 410 expired (Expired): the item's current state either way.
type InboxAnswerResult struct {
	Item    InboxItem
	Already bool
	Expired bool
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

// MaxAnswerRetryAfter caps the one wait after a 429 of the answer call.
const MaxAnswerRetryAfter = 5 * time.Second

// ActorUsernameRe is what GAOS accepts as actor.username (v0.43.0); "operator" is refused too.
var ActorUsernameRe = regexp.MustCompile(`^[A-Za-z0-9._@+-]{1,64}$`)

// ValidActorUsername reports whether GAOS would accept u as actor.username.
func ValidActorUsername(u string) bool { return ActorUsernameRe.MatchString(u) && u != "operator" }

// AnswerInboxItem answers the item in GAOS (HISTORY_ANSWER.md, GAOS v0.43.0). 200 is our
// answer; 409 already_answered and 410 expired carry the item's current state (the full GET
// shape), returned with Already / Expired set. A 429 is retried once after Retry-After (capped
// at MaxAnswerRetryAfter); every other status is a *Failure. The caller sets the time budget
// through ctx (the client's own limit is requestTimeout).
func (s *Syncer) AnswerInboxItem(ctx context.Context, gaosPID, externalID string, a InboxAnswer) (*InboxAnswerResult, error) {
	if err := s.inboxReady(gaosPID, externalID); err != nil {
		return nil, err
	}
	if a.Choice < 1 || !ValidActorUsername(a.Actor.Username) {
		return nil, errors.New("an answer needs a choice and a valid actor username")
	}
	payload, err := json.Marshal(a)
	if err != nil {
		return nil, &Failure{Note: "inbox answer: could not encode the answer"}
	}
	path := "/products/" + url.PathEscape(gaosPID) + "/inbox/" + url.PathEscape(externalID) + "/answer"
	var status int
	var data []byte
	for attempt := 0; ; attempt++ {
		var retryAfter time.Duration
		var f *Failure
		status, retryAfter, data, f = s.cl.once(ctx, path, payload)
		if f != nil {
			f.Note = "inbox answer: " + f.Note
			return nil, f
		}
		if status != http.StatusTooManyRequests || attempt > 0 {
			break
		}
		if retryAfter > MaxAnswerRetryAfter {
			retryAfter = MaxAnswerRetryAfter
		}
		if err := s.cl.sleep(ctx, retryAfter); err != nil {
			return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered HTTP 429 and the time budget ran out"}
		}
	}
	switch status {
	case http.StatusOK, http.StatusConflict, http.StatusGone:
		var it InboxItem
		if err := json.Unmarshal(data, &it); err != nil || it.ExternalID != externalID {
			return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered with an unexpected body"}
		}
		return &InboxAnswerResult{Item: it, Already: status == http.StatusConflict, Expired: status == http.StatusGone}, nil
	case http.StatusNotFound:
		return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered HTTP 404 (GAOS does not know this item, or has no answer endpoint yet)"}
	case http.StatusBadRequest:
		return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered HTTP 400 (" + errorCode(data) + ")"}
	case http.StatusUnprocessableEntity:
		return nil, &Failure{Status: status, Note: "inbox answer: GAOS answered HTTP 422 (" + errorCode(data) + ")"}
	}
	note := fmt.Sprintf("inbox answer: GAOS answered HTTP %d", status)
	if h, ok := hints[status]; ok && status != http.StatusNotFound {
		note += " (" + h + ")"
	}
	return nil, &Failure{Status: status, Note: note}
}

var errorCodeRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// errorCode is GAOS's short error code ("bad-actor", "bad-choice") when the body has a safe one,
// else "refused". Never the raw body.
func errorCode(data []byte) string {
	var b struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &b) == nil && errorCodeRe.MatchString(b.Error) {
		return b.Error
	}
	return "refused"
}
