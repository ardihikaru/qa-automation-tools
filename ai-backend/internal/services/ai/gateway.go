package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GatewayVendor is one provider/model row of ai-manager's GET /admin/usage.
type GatewayVendor struct {
	Vendor           string  `json:"vendor"`
	VendorType       string  `json:"vendor_type"`
	Model            string  `json:"model"`
	Calls            int     `json:"calls"`
	Errors           int     `json:"errors"`
	InputTokens      int64   `json:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	CostIDR          float64 `json:"cost_idr"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	EstimatedCostIDR float64 `json:"estimated_cost_idr"`
}

// GatewayUsage is ai-manager's monthly usage aggregate.
type GatewayUsage struct {
	Period      string          `json:"period"`
	Vendors     []GatewayVendor `json:"vendors"`
	TotalCalls  int             `json:"total_calls"`
	TotalErrors int             `json:"total_errors"`
}

// GatewayCall is one raw ai-manager usage row (GET /admin/usage/recent).
type GatewayCall struct {
	TS                string  `json:"ts"`
	Vendor            string  `json:"vendor"`
	Model             string  `json:"model"`
	RequestedProvider string  `json:"requested_provider"`
	Status            string  `json:"status"`
	Error             string  `json:"error"`
	Fallback          bool    `json:"fallback"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CostUSD           float64 `json:"cost_usd"`
	CostIDR           float64 `json:"cost_idr"`
	CostSource        string  `json:"cost_source"`
	LatencyMS         int64   `json:"latency_ms"`
	ClientIP          string  `json:"client_ip"`
}

// GatewayReader reads usage logs from ai-manager. Only the ai-manager provider implements it.
type GatewayReader interface {
	Usage(ctx context.Context, month string) (*GatewayUsage, error)
	Recent(ctx context.Context, limit int) ([]GatewayCall, error)
}

// Gateway returns the ai-manager usage reader, or nil for other providers / unconfigured.
func (s *Service) Gateway() GatewayReader {
	if s == nil || s.provider == nil || !s.provider.Configured() {
		return nil
	}
	g, _ := s.provider.(GatewayReader)
	return g
}

func (p *aiManagerProvider) adminGet(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if !p.Configured() {
		return nil, fmt.Errorf("ai-manager not configured")
	}
	u := p.cfg.URL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	// Admin reads are cheap; do not wait the full CLI chat timeout.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(p.cfg.LoginID, p.cfg.SecretKey)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai-manager: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("ai-manager: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
			if len(e.Error) > 200 {
				e.Error = e.Error[:200]
			}
		}
		return nil, fmt.Errorf("ai-manager: status %d: %s", resp.StatusCode, e.Error)
	}
	return raw, nil
}

func (p *aiManagerProvider) Usage(ctx context.Context, month string) (*GatewayUsage, error) {
	q := url.Values{}
	if month != "" {
		q.Set("month", month)
	}
	raw, err := p.adminGet(ctx, "/admin/usage", q)
	if err != nil {
		return nil, err
	}
	var out GatewayUsage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai-manager: bad usage payload: %w", err)
	}
	if out.Vendors == nil {
		out.Vendors = []GatewayVendor{}
	}
	return &out, nil
}

func (p *aiManagerProvider) Recent(ctx context.Context, limit int) ([]GatewayCall, error) {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	raw, err := p.adminGet(ctx, "/admin/usage/recent", q)
	if err != nil {
		return nil, err
	}
	return ParseGatewayCalls(raw)
}

// ParseGatewayCalls decodes /admin/usage/recent. ai-manager's usage.Entry has no JSON tags (keys
// are Go field names such as "InputTokens") while its docs say snake/camel case, so keys are
// matched with case and underscores ignored.
func ParseGatewayCalls(raw []byte) ([]GatewayCall, error) {
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("ai-manager: bad recent payload: %w", err)
	}
	out := make([]GatewayCall, 0, len(rows))
	for _, row := range rows {
		m := make(map[string]any, len(row))
		for k, v := range row {
			m[strings.ToLower(strings.ReplaceAll(k, "_", ""))] = v
		}
		str := func(k string) string { s, _ := m[k].(string); return s }
		num := func(k string) float64 { f, _ := m[k].(float64); return f }
		fb, _ := m["fallback"].(bool)
		out = append(out, GatewayCall{
			TS: str("ts"), Vendor: str("vendor"), Model: str("model"),
			RequestedProvider: str("requestedprovider"), Status: str("status"), Error: str("error"),
			Fallback: fb, InputTokens: int(num("inputtokens")), OutputTokens: int(num("outputtokens")),
			CostUSD: num("costusd"), CostIDR: num("costidr"), CostSource: str("costsource"),
			LatencyMS: int64(num("latencyms")), ClientIP: str("clientip"),
		})
	}
	return out, nil
}
