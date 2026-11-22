/**
 * Executor configuration. Environment only (no config.yaml): the systemd unit
 * loads it from an EnvironmentFile. Secrets are never logged.
 */
export interface ExecutorConfig {
  bind: string;
  port: number;
  token: string;
  runTimeoutMs: number;
  maxQueue: number;
  /** case_run: directory holding the automation repo clones. */
  repoPool: string;
  /** case_run: directory holding automation-*.env files (mode 600). */
  envDir: string;
  /** case_run: hostnames a case env file's base URLs may point at (`*.suffix` allowed). */
  allowedHosts: string[];
  /** case_run: absolute path of the navigation reporter, or '' (off, missing or not loadable). */
  navReporter?: string;
  /** case_run: Brain Town reporting (docs/designs/BRAIN_TOWN.md); off unless BRAIN_TOWN_URL is set. */
  brainTown?: BrainTownConfig;
}

import { BrainTownConfig, loadBrainTownConfig } from './brainTownReporter';
import { resolveNavReporter } from './navReporter';

type Env = Record<string, string | undefined>;

function int(env: Env, key: string, def: number, min: number, max: number): number {
  const raw = env[key];
  if (raw === undefined || raw === '') return def;
  const n = Number.parseInt(raw, 10);
  if (!Number.isFinite(n) || n < min || n > max) {
    throw new Error(`${key} must be an integer in [${min}, ${max}]`);
  }
  return n;
}

export function loadConfig(env: Env = process.env): ExecutorConfig {
  const token = env.EXECUTOR_TOKEN ?? '';
  if (token.length < 32) {
    throw new Error('EXECUTOR_TOKEN must be set (>= 32 chars)');
  }
  // Optional: HISV3_STAGING_URL only seeds the default allowed host list.
  const stagingUrl = (env.HISV3_STAGING_URL ?? '').trim().replace(/\/+$/, '');
  if (stagingUrl && !/^https?:\/\//.test(stagingUrl)) {
    throw new Error('HISV3_STAGING_URL must be an http(s) URL when set');
  }

  const allowedHosts = (env.EXECUTOR_ALLOWED_HOSTS ?? (stagingUrl ? new URL(stagingUrl).hostname : ''))
    .split(',')
    .map((h) => h.trim().toLowerCase())
    .filter(Boolean);

  return {
    bind: env.EXECUTOR_BIND || '127.0.0.1',
    port: int(env, 'EXECUTOR_PORT', 8141, 1, 65535),
    token,
    runTimeoutMs: int(env, 'RUN_TIMEOUT_SECONDS', 900, 30, 7200) * 1000,
    maxQueue: int(env, 'MAX_QUEUE', 10, 0, 100),
    repoPool: env.EXECUTOR_REPO_POOL || '/home/sismedika/devel/projects/qa-automation-tools/repos',
    envDir: env.EXECUTOR_ENV_DIR || `${env.HOME ?? '/home/sismedika'}/.config/qa-automation`,
    allowedHosts,
    navReporter: resolveNavReporter(env),
    brainTown: loadBrainTownConfig(env),
  };
}
