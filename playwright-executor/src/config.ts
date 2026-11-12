/**
 * Executor configuration. Environment only (no config.yaml): the systemd unit
 * loads it from an EnvironmentFile. Secrets are never logged.
 */
import * as fs from 'fs';

export interface Credential {
  username: string;
  password: string;
  role_code?: string;
}

export interface ExecutorConfig {
  bind: string;
  port: number;
  token: string;
  stagingUrl: string;
  credentials: Record<string, Credential>;
  defaultRoles: string[];
  outputDir: string;
  retentionDays: number;
  runTimeoutMs: number;
  maxQueue: number;
  headless: boolean;
}

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

export function parseCredentials(json: string): Record<string, Credential> {
  const parsed = JSON.parse(json) as unknown;
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('credentials file must be a JSON object keyed by role');
  }
  const out: Record<string, Credential> = {};
  for (const [role, v] of Object.entries(parsed as Record<string, unknown>)) {
    const c = v as Partial<Credential> | null;
    if (c && typeof c.username === 'string' && typeof c.password === 'string' && c.username && c.password) {
      out[role] = { username: c.username, password: c.password, role_code: c.role_code };
    }
  }
  return out;
}

export function loadConfig(env: Env = process.env): ExecutorConfig {
  const token = env.EXECUTOR_TOKEN ?? '';
  if (token.length < 32) {
    throw new Error('EXECUTOR_TOKEN must be set (>= 32 chars)');
  }
  const stagingUrl = (env.TARGET_STAGING_URL ?? '').trim().replace(/\/+$/, '');
  if (!/^https?:\/\//.test(stagingUrl)) {
    throw new Error('TARGET_STAGING_URL must be an http(s) URL of SUT staging');
  }
  const credFile = env.TARGET_CREDENTIALS_FILE ?? '';
  if (!credFile) throw new Error('TARGET_CREDENTIALS_FILE must be set');
  const credentials = parseCredentials(fs.readFileSync(credFile, 'utf8'));
  if (Object.keys(credentials).length === 0) {
    throw new Error('TARGET_CREDENTIALS_FILE has no usable role credentials');
  }

  const defaultRoles = (env.EXECUTOR_DEFAULT_ROLES ?? 'admin,doctor_gp,cashier')
    .split(',')
    .map((s) => s.trim())
    .filter((r) => r && credentials[r]);

  return {
    bind: env.EXECUTOR_BIND || '127.0.0.1',
    port: int(env, 'EXECUTOR_PORT', 8141, 1, 65535),
    token,
    stagingUrl,
    credentials,
    defaultRoles: defaultRoles.length ? defaultRoles : [Object.keys(credentials)[0]],
    outputDir: env.EXECUTOR_OUTPUT_DIR || `${process.cwd()}/output/runs`,
    retentionDays: int(env, 'EXECUTOR_RETENTION_DAYS', 7, 1, 365),
    runTimeoutMs: int(env, 'RUN_TIMEOUT_SECONDS', 900, 30, 7200) * 1000,
    maxQueue: int(env, 'MAX_QUEUE', 10, 0, 100),
    headless: (env.EXECUTOR_HEADLESS ?? 'true') !== 'false',
  };
}
