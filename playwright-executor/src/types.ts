export type RunType = 'qa_run' | 'docs_generate';
export type RunStatus = 'queued' | 'running' | 'completed' | 'failed';

export interface RunParams {
  roles: string[];
  max_steps: number;
  /** Run skills that click/submit (required_field). Staging only, off by default. */
  interactive_skills: boolean;
  /** docs_generate: restrict to these flow ids (empty = all flows of each role). */
  flows: string[];
}

export interface RunRecord {
  id: string;
  job_id: string;
  type: RunType;
  params: RunParams;
  status: RunStatus;
  result?: Record<string, unknown>;
  error?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
}

export const RUN_TYPES: readonly RunType[] = ['qa_run', 'docs_generate'];
export const MAX_STEPS_LIMIT = 50;
export const MAX_ROLES = 5;
