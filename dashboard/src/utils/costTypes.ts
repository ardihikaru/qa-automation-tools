// Response shapes of GET /api/cost/*. Day buckets are UTC.
// Note: utils/mockDataGenerator.ts keeps its own minimal `CostOverview`
// for QAInsights; this one is the full API shape.

export interface CostOverview {
  total_requests: number;
  success_calls: number;
  error_calls: number;
  error_rate: number; // 0..1
  fallback_calls: number;
  total_tokens: number;
  input_tokens: number;
  output_tokens: number;
  total_cost: number; // USD
  total_cost_idr: number;
  estimated_cost: number; // USD, notional payg_estimate, not real spend
  avg_latency_ms: number;
  p95_latency_ms: number;
}

export interface CostTrendPoint {
  date: string;
  cost: number;
  cost_idr: number;
  calls: number;
  errors: number;
}

export interface CostByFeature {
  feature: string;
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  cost: number;
  cost_idr: number;
}

export interface CostByModel {
  provider: string;
  model: string;
  vendor?: string; // derived by the backend, e.g. "Anthropic"
  model_label?: string; // e.g. "Haiku 5.5"
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  cost: number;
  cost_idr: number;
}

export interface CostEfficiency {
  calls: number;
  success: number;
  errors: number;
  fallbacks: number;
}

export interface CostRecentItem {
  id: number | string;
  created_at: string;
  feature: string;
  caller: string;
  provider: string;
  model: string;
  status: 'ok' | 'error';
  error?: string;
  fallback: boolean;
  input_tokens: number;
  output_tokens: number;
  cost: number;
  cost_idr: number;
  latency_ms: number;
}

export interface CostRecent {
  items: CostRecentItem[];
  total: number;
  limit: number;
  offset: number;
}

export interface CostFilterOptions {
  features: string[];
  providers: string[];
  models: string[];
}

export interface GatewayVendor {
  vendor: string;
  vendor_type: string;
  model: string;
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  cost_idr: number;
  estimated_cost_usd: number;
  estimated_cost_idr: number;
}

export interface GatewayRecent {
  ts: string;
  vendor: string;
  model: string;
  requested_provider: string;
  status: string;
  error?: string;
  fallback: boolean;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  cost_idr: number;
  cost_source: string;
  latency_ms: number;
  client_ip: string;
}

export interface CostGateway {
  month: string;
  local?: { calls: number; errors: number; cost: number; cost_idr: number };
  available: boolean;
  error?: string;
  usage?: {
    period: string;
    vendors: GatewayVendor[];
    total_calls: number;
    total_errors: number;
  };
  recent?: GatewayRecent[];
}

// GET /api/cost/by-vendor
export interface CostVendorFeature {
  feature: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cost: number;
  cost_idr: number;
}

export interface CostVendorModel {
  provider: string;
  model: string;
  label: string;
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  cost: number;
  cost_idr: number;
  avg_cost_per_call: number;
  avg_cost_idr_per_call: number;
  share_pct: number; // of total cost (of calls when total cost is 0)
  features: CostVendorFeature[];
}

export interface CostVendor {
  vendor: string;
  calls: number;
  errors: number;
  input_tokens: number;
  output_tokens: number;
  cost: number;
  cost_idr: number;
  avg_cost_per_call: number;
  avg_cost_idr_per_call: number;
  share_pct: number;
  models: CostVendorModel[];
}

export interface CostByVendor {
  total_cost: number;
  total_cost_idr: number;
  total_calls: number;
  vendors: CostVendor[];
}
