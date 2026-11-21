// Types and fetchers of the learning loop (docs/designs/LEARNING_LOOP.md,
// ai-backend/internal/api/learning_loop.go). The server stays the authority.
import { api, ApiError } from '../products/api';

export { ApiError };

export type FindingStatus = 'new' | 'proposed' | 'learned' | 'covered' | 'dismissed';
export type FindingKind = 'review_changes' | 'review_rejected' | 'repair_fixed' | 'repeated_failure' | 'manual';
export type ProposalState =
  | 'drafting'
  | 'awaiting_approval'
  | 'publishing'
  | 'pr_open'
  | 'learned'
  | 'covered'
  | 'rejected'
  | 'closed'
  | 'failed';

export const FINDING_STATUSES: FindingStatus[] = ['new', 'proposed', 'learned', 'covered', 'dismissed'];
export const FINDING_KINDS: FindingKind[] = ['review_changes', 'review_rejected', 'repair_fixed', 'repeated_failure', 'manual'];
export const PROPOSAL_STATES: ProposalState[] = ['drafting', 'awaiting_approval', 'publishing', 'pr_open', 'learned', 'covered', 'rejected', 'closed', 'failed'];

export const KIND_LABEL: Record<FindingKind, string> = {
  review_changes: 'Reviewer asked for changes',
  review_rejected: 'Reviewer rejected a spec',
  repair_fixed: 'Repair fixed a failure',
  repeated_failure: 'Repeated failure',
  manual: 'Entered by hand',
};

export const FINDING_STATUS_LABEL: Record<FindingStatus, string> = {
  new: 'new',
  proposed: 'in a proposal',
  learned: 'learned',
  covered: 'already covered',
  dismissed: 'dismissed',
};

export const STATE_LABEL: Record<ProposalState, string> = {
  drafting: 'drafting',
  awaiting_approval: 'awaiting approval',
  publishing: 'opening PR',
  pr_open: 'PR open',
  learned: 'learned',
  covered: 'already covered',
  rejected: 'rejected',
  closed: 'PR closed',
  failed: 'failed',
};

export const STATE_CLASS: Record<ProposalState, string> = {
  drafting: 'bg-blue-100 text-blue-800',
  awaiting_approval: 'bg-amber-100 text-amber-800',
  publishing: 'bg-blue-100 text-blue-800',
  pr_open: 'bg-indigo-100 text-indigo-800',
  learned: 'badge-success',
  covered: 'bg-gray-100 text-gray-700',
  rejected: 'bg-gray-100 text-gray-700',
  closed: 'bg-gray-100 text-gray-700',
  failed: 'badge-error',
};

export const FINDING_STATUS_CLASS: Record<FindingStatus, string> = {
  new: 'bg-amber-100 text-amber-800',
  proposed: 'bg-blue-100 text-blue-800',
  learned: 'badge-success',
  covered: 'bg-gray-100 text-gray-700',
  dismissed: 'bg-gray-100 text-gray-500',
};

/** Proposal states in which the server is still working (the page polls). */
export const BUSY_STATES: ProposalState[] = ['drafting', 'publishing'];

export interface Source {
  job_id: string;
  case_code: string;
}

export interface Finding {
  id: number;
  product_id: number;
  product_name: string;
  kind: FindingKind;
  status: FindingStatus;
  title: string;
  detail: string;
  occurrences: number;
  first_seen_at: string;
  last_seen_at: string;
  updated_at: string;
  proposal_job_id: string;
  created_by: string;
  status_note: string;
  sources: Source[];
}

export interface Evidence {
  source_key: string;
  job_id: string;
  case_code: string;
  run_ref: string;
  excerpt: string;
  observed_at: string;
}

export interface Proposal {
  id: string;
  product_id: number;
  product_name: string;
  state: ProposalState;
  status: string;
  created_at: string;
  completed_at: string | null;
  triggered_by: string;
  finding_ids: number[];
  title: string;
  slug: string;
  file_path: string;
  policy: string;
  rationale: string;
  covered_by: string;
  content: string;
  branch: string;
  pr_url: string;
  pr_state: string;
  merge_commit: string;
  merged_at: string;
  pr_checked_at: string;
  in_repo: boolean;
  error: string;
}

export interface TimelineItem {
  at: string;
  kind: string;
  product_id: number;
  product_name: string;
  finding_id?: number;
  finding_kind?: FindingKind;
  job_id?: string;
  title: string;
  state?: ProposalState;
  pr_url?: string;
  file_path?: string;
  case_codes?: string[];
  occurrences?: number;
}

export interface Page<T, K extends string> {
  items: T[];
  total: number;
  counts: Record<K, number>;
}

export interface ProductOption {
  id: number;
  name: string;
  hasAutomation: boolean;
}

const qs = (params: Record<string, string | number | undefined>) => {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== '' && v !== 0) u.set(k, String(v));
  const s = u.toString();
  return s ? `?${s}` : '';
};

export const listFindings = (p: { product_id?: number; status?: string; kind?: string; q?: string; limit?: number; offset?: number }, signal?: AbortSignal) =>
  api<Page<Finding, FindingStatus>>(`/api/learning/findings${qs(p)}`, { signal });

export const getFinding = (id: number, signal?: AbortSignal) =>
  api<{ finding: Finding; evidence: Evidence[] }>(`/api/learning/findings/${id}`, { signal });

export const addFinding = (body: { product_id: number; title: string; detail: string; case_code?: string; job_id?: string; run_ref?: string }) =>
  api<{ finding: Finding; merged: boolean }>('/api/learning/findings', { method: 'POST', body: JSON.stringify(body) });

export const setFindingStatus = (id: number, action: 'dismiss' | 'reopen', reason?: string) =>
  api<Finding>(`/api/learning/findings/${id}/${action}`, { method: 'POST', body: JSON.stringify(reason ? { reason } : {}) });

export const listProposals = (p: { product_id?: number; state?: string; limit?: number; offset?: number }, signal?: AbortSignal) =>
  api<Page<Proposal, ProposalState>>(`/api/learning/proposals${qs(p)}`, { signal });

export const createProposal = (product_id: number, finding_ids: number[]) =>
  api<{ job_id: string }>('/api/learning/proposals', { method: 'POST', body: JSON.stringify({ product_id, finding_ids }) });

export const checkProposal = (id: string) => api<Proposal>(`/api/learning/proposals/${encodeURIComponent(id)}/check`, { method: 'POST', body: '{}' });

/** The existing approval door of a job (CASE_AUTOMATION.md): approve, reject or ask for changes. */
export const answerProposal = (id: string, decision: 'approve' | 'reject' | 'changes', text?: string) =>
  api<{ status: string }>(`/api/jobs/${encodeURIComponent(id)}/approval`, { method: 'POST', body: JSON.stringify(text ? { decision, text } : { decision }) });

export const getTimeline = (p: { product_id?: number; limit?: number; offset?: number }, signal?: AbortSignal) =>
  api<{ items: TimelineItem[]; total: number }>(`/api/learning/timeline${qs(p)}`, { signal });

export async function listProducts(signal?: AbortSignal): Promise<ProductOption[]> {
  const r = await api<{ products?: { id: number; name: string; repos?: { is_automation: boolean }[] }[] }>('/api/products', { signal });
  return (r.products ?? []).map((p) => ({ id: p.id, name: p.name, hasAutomation: (p.repos ?? []).some((x) => x.is_automation) }));
}

export function errorText(e: unknown, fallback = 'Request failed'): string {
  if (e instanceof ApiError) return e.message;
  return e instanceof Error ? e.message : fallback;
}

/** Rows per page on every /learning tab. */
export const LEARNING_PAGE_SIZE = 10;

export const isAbort = (e: unknown) => e instanceof DOMException && e.name === 'AbortError';

/** Writers of learning data (mirrors productRole's canVerify). */
export const canWriteLearning = (role: string | undefined) => ['Admin', 'DevOps', 'Developer', 'QA', 'QC'].includes(role ?? '');

export const jobHref = (id: string) => `/jobs/${encodeURIComponent(id)}`;
export const caseHref = (pid: number, code: string) => `/products/${pid}/knowledge/test-cases/${encodeURIComponent(code)}`;

/** A PR link only when it is a plain GitHub pull request URL. */
export const safePR = (u: string | undefined) => (u && /^https:\/\/github\.com\/[\w.-]+\/[\w.-]+\/pull\/\d+$/.test(u) ? u : '');

export function fmtDate(iso: string | null | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

export function fmtDay(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { weekday: 'short', year: 'numeric', month: 'short', day: 'numeric' });
}
