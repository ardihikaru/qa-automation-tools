package ai

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// CallRecord is one AI call attempt, written to ai_call_log for the /cost page.
type CallRecord struct {
	At                time.Time
	Feature           string
	Caller            string
	Provider          string
	Model             string
	RequestedProvider string
	Status            string // "ok" | "error"
	Error             string
	Fallback          bool
	InputTokens       int
	OutputTokens      int
	CostUSD           float64
	CostIDR           float64
	CostSource        string
	LatencyMS         int
}

// CallRecorder persists call records. Implementations must not block the caller for long.
type CallRecorder interface {
	RecordCall(rec CallRecord)
}

// SQLRecorder writes call records to the ai_call_log table. Failures are logged only:
// accounting must never break or slow an AI call.
type SQLRecorder struct{ DB *sql.DB }

func (r *SQLRecorder) RecordCall(rec CallRecord) {
	if r == nil || r.DB == nil {
		return
	}
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	if rec.Feature == "" {
		rec.Feature = "unknown"
	}
	if len(rec.Error) > 1000 {
		rec.Error = rec.Error[:1000]
	}
	var errVal any
	if rec.Error != "" {
		errVal = rec.Error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.DB.ExecContext(ctx, `INSERT INTO ai_call_log
(created_at, feature, caller, provider, model, requested_provider, status, error, fallback,
 input_tokens, output_tokens, cost_usd, cost_idr, cost_source, latency_ms)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rec.At, rec.Feature, rec.Caller, rec.Provider, rec.Model, rec.RequestedProvider, rec.Status, errVal,
		rec.Fallback, rec.InputTokens, rec.OutputTokens, rec.CostUSD, rec.CostIDR, rec.CostSource, rec.LatencyMS)
	if err != nil {
		log.Printf("ai_call_log insert failed: %v", err)
	}
}
