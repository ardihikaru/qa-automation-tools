import {
  CASE_CODE_RE,
  CaseError,
  checkSpecShape,
  ENV_FILE_RE,
  listEnvFiles,
  MAX_SPECS,
  resolveRepoDir,
  resolveSpecs,
} from './caseFs';
import { CaseRunParams, MAX_ROLES, MAX_STEPS_LIMIT, RUN_TYPES, RunParams, RunType } from './types';

export interface RunRequest {
  job_id: string;
  type: RunType;
  params: RunParams | CaseRunParams;
}

/** Where case_run params are checked against the filesystem. */
export interface CaseEnv {
  repoPool: string;
  envDir: string;
}

const CASE_KEYS = ['repo_dir', 'case_code', 'spec_paths', 'env_file', 'repair'];

/** Strict case_run validation: unknown keys are rejected, paths are realpath-confined. */
export function parseCaseParams(p: Record<string, unknown>, env: CaseEnv): { ok: true; value: CaseRunParams } | { ok: false; error: string } {
  const extra = Object.keys(p).filter((k) => !CASE_KEYS.includes(k));
  if (extra.length) return { ok: false, error: `unknown params: ${extra.join(', ')}` };
  try {
    if (typeof p.repo_dir !== 'string') throw new CaseError('params.repo_dir must be a string');
    if (typeof p.case_code !== 'string' || !CASE_CODE_RE.test(p.case_code)) throw new CaseError('params.case_code must match ^TC-[A-Z0-9]+-[0-9]{3,4}$');
    if (!Array.isArray(p.spec_paths) || p.spec_paths.length < 1 || p.spec_paths.length > MAX_SPECS) {
      throw new CaseError(`params.spec_paths must be an array of 1..${MAX_SPECS} paths`);
    }
    const specs = p.spec_paths.map(checkSpecShape);
    if (new Set(specs).size !== specs.length) throw new CaseError('params.spec_paths has duplicates');
    if (typeof p.env_file !== 'string' || !ENV_FILE_RE.test(p.env_file) || !listEnvFiles(env.envDir).includes(p.env_file)) {
      throw new CaseError('params.env_file must be the name of an env file in EXECUTOR_ENV_DIR');
    }
    let repair: number | undefined;
    if (p.repair !== undefined) {
      if (typeof p.repair !== 'number' || !Number.isInteger(p.repair) || p.repair < 0 || p.repair > 99) {
        throw new CaseError('params.repair must be an integer in [0, 99]');
      }
      repair = p.repair;
    }
    const repo = resolveRepoDir(p.repo_dir, env.repoPool);
    resolveSpecs(repo, specs);
    return { ok: true, value: { repo_dir: p.repo_dir, case_code: p.case_code, spec_paths: specs, env_file: p.env_file, ...(repair !== undefined ? { repair } : {}) } };
  } catch (e) {
    if (e instanceof CaseError) return { ok: false, error: e.message };
    throw e;
  }
}

export type Parsed = { ok: true; value: RunRequest } | { ok: false; error: string };

/**
 * Validates a POST /v1/runs body. The target URL is never taken from the
 * request: runs always go to the configured HISv3 staging URL.
 */
export function parseRunRequest(
  body: unknown,
  knownRoles: string[],
  defaultRoles: string[],
  caseEnv?: CaseEnv,
): Parsed {
  if (!body || typeof body !== 'object' || Array.isArray(body)) {
    return { ok: false, error: 'body must be a JSON object' };
  }
  const b = body as Record<string, unknown>;

  const type = b.type;
  if (typeof type !== 'string' || !RUN_TYPES.includes(type as RunType)) {
    return { ok: false, error: `type must be one of ${RUN_TYPES.join(', ')}` };
  }

  const jobId = b.job_id ?? '';
  if (typeof jobId !== 'string' || jobId.length > 128) {
    return { ok: false, error: 'job_id must be a string of at most 128 chars' };
  }

  const p = (b.params ?? {}) as Record<string, unknown>;
  if (typeof p !== 'object' || Array.isArray(p)) {
    return { ok: false, error: 'params must be an object' };
  }

  if (type === 'case_run') {
    if (!caseEnv) return { ok: false, error: 'case_run is not configured' };
    if (!b.params || typeof b.params !== 'object' || Array.isArray(b.params)) return { ok: false, error: 'params must be an object' };
    const c = parseCaseParams(b.params as Record<string, unknown>, caseEnv);
    return c.ok ? { ok: true, value: { job_id: jobId, type: 'case_run', params: c.value } } : c;
  }

  let roles = defaultRoles;
  if (p.roles !== undefined) {
    if (!Array.isArray(p.roles) || p.roles.some((r) => typeof r !== 'string')) {
      return { ok: false, error: 'params.roles must be an array of strings' };
    }
    roles = [...new Set(p.roles as string[])];
    const unknown = roles.filter((r) => !knownRoles.includes(r));
    if (unknown.length) return { ok: false, error: `unknown roles: ${unknown.join(', ')}` };
    if (roles.length === 0) roles = defaultRoles;
  }
  if (roles.length > MAX_ROLES) {
    return { ok: false, error: `at most ${MAX_ROLES} roles per run` };
  }

  let maxSteps = 10;
  if (p.max_steps !== undefined) {
    if (typeof p.max_steps !== 'number' || !Number.isInteger(p.max_steps) || p.max_steps < 1 || p.max_steps > MAX_STEPS_LIMIT) {
      return { ok: false, error: `params.max_steps must be an integer in [1, ${MAX_STEPS_LIMIT}]` };
    }
    maxSteps = p.max_steps;
  }

  let flows: string[] = [];
  if (p.flows !== undefined) {
    if (!Array.isArray(p.flows) || p.flows.some((f) => typeof f !== 'string')) {
      return { ok: false, error: 'params.flows must be an array of strings' };
    }
    flows = p.flows as string[];
  }

  return {
    ok: true,
    value: {
      job_id: jobId,
      type: type as RunType,
      params: { roles, max_steps: maxSteps, interactive_skills: p.interactive_skills === true, flows },
    },
  };
}
