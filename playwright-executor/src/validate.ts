import { MAX_ROLES, MAX_STEPS_LIMIT, RUN_TYPES, RunParams, RunType } from './types';

export interface RunRequest {
  job_id: string;
  type: RunType;
  params: RunParams;
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
