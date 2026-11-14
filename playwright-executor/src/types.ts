export type RunType = 'case_run';
export type RunStatus = 'queued' | 'running' | 'completed' | 'failed';

/** case_run params (validated in validate.ts, nothing else is accepted). */
export interface CaseRunParams {
  repo_dir: string;
  case_code: string;
  spec_paths: string[];
  env_file: string;
  repair?: number;
}

export type RunEventLevel = 'info' | 'warn' | 'error';

/** One progress line. `n` is monotonic per run, starting at 1. */
export interface RunEvent {
  n: number;
  at: string;
  level: RunEventLevel;
  step: string;
  message: string;
}

export const EVENT_RING = 500;

export interface RunRecord {
  id: string;
  job_id: string;
  type: RunType;
  params: CaseRunParams;
  status: RunStatus;
  /** Ring of the last {@link EVENT_RING} events. GET ?since=n returns only n > since. */
  events: RunEvent[];
  result?: Record<string, unknown>;
  error?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
}

export const RUN_TYPES: readonly RunType[] = ['case_run'];
