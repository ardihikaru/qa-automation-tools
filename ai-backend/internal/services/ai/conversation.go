package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/qa-automation/ai-backend/internal/models"
)

// Conversation kinds (AI_CONVERSATIONS.md 3.2).
const (
	ConvJob  = "job"
	ConvSync = "sync"
	ConvCall = "call"
)

// ConvRef names the work a call belongs to: a case_create job, a knowledge sync, or a call of its own.
type ConvRef struct {
	Kind      string
	Ref       string
	ProductID int
	Title     string
}

// Exchange is one call as the conversation store receives it. Reply is empty when Err is set.
type Exchange struct {
	Ref      ConvRef
	Feature  string
	Provider string
	Model    string
	Sent     []models.Message
	Reply    string
	Err      error
}

// ConversationStore keeps every exchange. It must not change the call: errors are logged only.
type ConversationStore interface {
	SaveExchange(ctx context.Context, ex Exchange) error
}

// storeTimeout bounds how long a call waits for its exchange to be stored.
const storeTimeout = 5 * time.Second

func newCallRef(feature string) ConvRef {
	var b [2]byte
	_, _ = rand.Read(b[:])
	title := feature
	if title == "" {
		title = "unknown"
	}
	return ConvRef{Kind: ConvCall, Ref: fmt.Sprintf("call-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:])), Title: title}
}

// saveExchange hands the call to the store. A store error or panic is logged and never reaches
// the caller; the store gets its own deadline, detached from the caller's cancellation.
func saveExchange(ctx context.Context, cs ConversationStore, sent []models.Message, opts ChatOpts, res ChatResult, callErr error) {
	ref := newCallRef(opts.Feature)
	if opts.Conversation != nil && opts.Conversation.Kind != "" && opts.Conversation.Ref != "" {
		ref = *opts.Conversation
	}
	ex := Exchange{Ref: ref, Feature: opts.Feature, Provider: res.Provider, Model: res.Model,
		Sent: append([]models.Message(nil), sent...), Err: callErr}
	if callErr == nil {
		ex.Reply = res.Text
	}
	defer func() {
		if p := recover(); p != nil {
			log.Printf("ai conversations: %s/%s: store panicked: %v", ref.Kind, ref.Ref, p)
		}
	}()
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	if err := cs.SaveExchange(sctx, ex); err != nil {
		log.Printf("ai conversations: %s/%s: %v", ref.Kind, ref.Ref, err)
	}
}
