// Run history (docs/designs/RUN_HISTORY.md, ai-backend/internal/api/history.go). Read-only.
import { useEffect, useState } from 'react';
import { api } from '../products/api';
import { errorText, isAbort } from '../knowledge/api';

export interface StepMs {
  step: string;
  ms: number;
}

export interface Build {
  id: string;
  type: 'case_create' | 'case_run' | string;
  status: string;
  error: string;
  product_id: number;
  product_name: string;
  case_code: string;
  epic_code: string;
  batch_id: string;
  pr_url: string;
  outcome: string;
  created_at: string;
  started_at: string | null;
  completed_at: string | null;
  queue_ms: number | null;
  active_ms: number | null;
  waiting_ms: number;
  machine_ms: number | null;
  approvals: number;
  runs: number;
  steps: StepMs[];
  /** The job's AI conversation (AI_CONVERSATIONS.md), null when none or archived. */
  conversation_id: number | null;
}

export interface BuildSummary {
  count: number;
  completed: number;
  failed: number;
  running: number;
  avg_active_ms: number | null;
  avg_waiting_ms: number | null;
  avg_machine_ms: number | null;
}

export interface Run {
  id: number;
  product_id: number;
  product_name: string;
  source: 'ci' | 'upload' | 'case-run' | 'case-create' | string;
  run_ref: string;
  job_id: string;
  case_code: string;
  run_index: number | null;
  started_at: string;
  duration_ms: number | null;
  tests: number;
  passed: number;
  failed: number;
  flaky: number;
  skipped: number;
  cases: number;
  outcome: 'pass' | 'fail' | 'skipped' | string;
  case_result?: string | null;
  case_duration_ms?: number | null;
}

export interface RunCase {
  case_code: string;
  epic_code: string;
  known: boolean;
  result: 'pass' | 'fail' | 'skipped' | string;
  duration_ms: number;
  tests: number;
  attempts: number;
}

export type RunDetail = Run & { case_rows: RunCase[] };

export interface RunSummary {
  count: number;
  pass: number;
  fail: number;
  skipped: number;
  avg_duration_ms: number | null;
}

export interface SyncItem {
  id: number;
  product_id: number;
  product_name: string;
  status: 'running' | 'succeeded' | 'failed' | 'skipped';
  trigger: string;
  repo_name: string;
  branch: string;
  commit_sha: string;
  started_at: string;
  finished_at: string | null;
  duration_ms: number | null;
  error: string;
  conversation_id: number | null;
}

export interface HistoryPage<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export type Filters = Record<string, string>;

function qs(f: Filters): string {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) u.set(k, v);
  const s = u.toString();
  return s ? `?${s}` : '';
}

export const listBuilds = (f: Filters, signal?: AbortSignal) =>
  api<HistoryPage<Build> & { summary: BuildSummary }>(`/api/history/builds${qs(f)}`, { signal });

export const listRuns = (f: Filters, signal?: AbortSignal) =>
  api<HistoryPage<Run> & { summary: RunSummary; since: string | null }>(`/api/history/runs${qs(f)}`, { signal });

export const getRun = (id: number, signal?: AbortSignal) => api<RunDetail>(`/api/history/runs/${id}`, { signal });

export const listSyncs = (f: Filters, signal?: AbortSignal) => api<HistoryPage<SyncItem>>(`/api/history/syncs${qs(f)}`, { signal });

/** Steps that are a human's time, not the machine's. */
export const WAITING_STEPS = new Set(['approval']);

export const STEP_COLOR: Record<string, string> = {
  prepare: 'bg-slate-400',
  validate: 'bg-slate-400',
  install: 'bg-slate-500',
  generate: 'bg-indigo-500',
  write: 'bg-indigo-300',
  run: 'bg-sky-500',
  repair: 'bg-violet-500',
  report: 'bg-sky-300',
  approval: 'bg-amber-400',
  push: 'bg-emerald-500',
  pr: 'bg-emerald-400',
  done: 'bg-gray-300',
};

export const OUTCOME_CLASS: Record<string, string> = {
  pass: 'badge-success',
  fail: 'badge-error',
  skipped: 'bg-amber-100 text-amber-800',
};

export const SOURCE_LABEL: Record<string, string> = {
  ci: 'CI upload',
  upload: 'Dashboard upload',
  'case-run': 'Run case',
  'case-create': 'Build run',
};

export const DEFAULT_PAGE_SIZE = 10;

/** A row's product for the Product column: its name, else "Product 7", else a dash. */
export const productLabel = (name: string, id: number | null | undefined) => name || (id ? `Product ${id}` : '–');

/** "1 run", "3 runs". */
export const plural = (n: number, noun: string) => `${n.toLocaleString()} ${noun}${n === 1 ? '' : 's'}`;

export interface TabProps {
  filters: Filters;
  setFilter: (key: string, value: string) => void;
  refreshKey: number;
}

/** Loads one list for the current filters; aborts on change. */
export function useHistoryList<T>(load: (f: Filters, s: AbortSignal) => Promise<T>, filters: Filters, refreshKey: number) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [retry, setRetry] = useState(0);
  const key = JSON.stringify(filters);
  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    load(JSON.parse(key) as Filters, ac.signal)
      .then((d) => setData(d))
      .catch((e) => {
        if (!isAbort(e) && !ac.signal.aborted) setError(errorText(e, 'Could not load the history'));
      })
      .finally(() => {
        if (!ac.signal.aborted) setLoading(false);
      });
    return () => ac.abort();
  }, [load, key, refreshKey, retry]);
  return { data, error, loading, retry: () => setRetry((n) => n + 1) };
}

