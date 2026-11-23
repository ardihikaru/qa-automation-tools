/**
 * Executor configuration. Environment only (no config.yaml): the systemd unit
 * loads it from an EnvironmentFile. Secrets are never logged.
 */
export interface ExecutorConfig {
  bind: string;
  port: number;
  token: string;
  /** Every accepted bearer token: EXECUTOR_TOKEN first, then EXECUTOR_EXTRA_TOKENS (one per backend instance). */
  tokens: string[];
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
  /** Instance name of each accepted token ('' = default instance). Tokens absent from the map are the default. */
  tokenInstances?: Record<string, string>;
  /**
   * case_run: Brain Town config of one instance ('' = default). Reporting is off unless that instance has its
   * own BRAIN_TOWN_URL (see playwright-executor/README.md); a named instance never uses the default's.
   */
  brainTownFor?: (instance: string) => BrainTownConfig;
}

export const INSTANCE_NAME = /^[a-z][a-z0-9_-]{0,31}$/i;

/** One EXECUTOR_EXTRA_TOKENS entry: `name:token` or a bare token (default instance). Tokens never contain ':'. */
export function parseExtraToken(entry: string): { instance: string; token: string } {
  const i = entry.indexOf(':');
  if (i < 0) {
    if (entry.length < 32) throw new Error('every EXECUTOR_EXTRA_TOKENS entry must have a token >= 32 chars');
    return { instance: '', token: entry };
  }
  const name = entry.slice(0, i).trim();
  const token = entry.slice(i + 1).trim();
  if (!INSTANCE_NAME.test(name)) {
    throw new Error('EXECUTOR_EXTRA_TOKENS instance names must match [a-z][a-z0-9_-]{0,31}');
  }
  if (token.length < 32) throw new Error('every EXECUTOR_EXTRA_TOKENS entry must have a token >= 32 chars');
  return { instance: name.toLowerCase(), token };
}

import { BRAIN_TOWN_REPORTER_PATH, BrainTownConfig, loadBrainTownConfig } from './brainTownReporter';
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
  // Optional: more tokens, comma-separated, so several qa-tool instances can share one executor
  // and each can be revoked on its own. `name:token` names the instance (own Brain Town config).
  const extra = (env.EXECUTOR_EXTRA_TOKENS ?? '').split(',').map((t) => t.trim()).filter(Boolean).map(parseExtraToken);
  const tokens = [token];
  const tokenInstances: Record<string, string> = {};
  for (const e of extra) {
    if (tokens.includes(e.token)) continue; // EXECUTOR_TOKEN (default) and the first entry win
    tokens.push(e.token);
    if (e.instance) tokenInstances[e.token] = e.instance;
  }
  const btFile = BRAIN_TOWN_REPORTER_PATH;
  const btCache = new Map<string, BrainTownConfig>();
  const brainTownFor = (instance: string): BrainTownConfig => {
    let c = btCache.get(instance);
    if (!c) btCache.set(instance, (c = loadBrainTownConfig(env, btFile, instance)));
    return c;
  };
  // Optional: TARGET_STAGING_URL only seeds the default allowed host list.
  const stagingUrl = (env.TARGET_STAGING_URL ?? '').trim().replace(/\/+$/, '');
  if (stagingUrl && !/^https?:\/\//.test(stagingUrl)) {
    throw new Error('TARGET_STAGING_URL must be an http(s) URL when set');
  }

  const allowedHosts = (env.EXECUTOR_ALLOWED_HOSTS ?? (stagingUrl ? new URL(stagingUrl).hostname : ''))
    .split(',')
    .map((h) => h.trim().toLowerCase())
    .filter(Boolean);

  return {
    bind: env.EXECUTOR_BIND || '127.0.0.1',
    port: int(env, 'EXECUTOR_PORT', 8141, 1, 65535),
    token,
    tokens,
    runTimeoutMs: int(env, 'RUN_TIMEOUT_SECONDS', 900, 30, 7200) * 1000,
    maxQueue: int(env, 'MAX_QUEUE', 10, 0, 100),
    repoPool: env.EXECUTOR_REPO_POOL || `${env.HOME ?? '/home/qa'}/devel/projects/qa-automation-tools/repos`,
    envDir: env.EXECUTOR_ENV_DIR || `${env.HOME ?? '/home/qa'}/.config/qa-automation`,
    allowedHosts,
    navReporter: resolveNavReporter(env),
    tokenInstances,
    brainTownFor,
  };
}
