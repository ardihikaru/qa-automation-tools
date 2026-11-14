export type RunType = 'qa_run' | 'docs_generate' | 'case_run';
export type RunStatus = 'queued' | 'running' | 'completed' | 'failed';

export interface RunParams {
  roles: string[];
  max_steps: number;
  /** Run skills that click/submit (required_field). Staging only, off by default. */
  interactive_skills: boolean;
  /** docs_generate: restrict to these flow ids (empty = all flows of each role). */
  flows: string[];
}

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
  params: RunParams | CaseRunParams;
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

export const RUN_TYPES: readonly RunType[] = ['qa_run', 'docs_generate', 'case_run'];
export const MAX_STEPS_LIMIT = 50;
export const MAX_ROLES = 5;
