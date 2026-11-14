/**
 * case_run: runs one test case of a product's automation repo (a clone inside
 * the repo pool) with `npx playwright test`, as a child process.
 *
 * Child processes get argument arrays (never a shell), a minimal environment
 * plus only the chosen env file's keys, and their own process group so an
 * abort/timeout kills the whole tree. stdout/stderr become redacted, clipped
 * events. No browser is launched by the executor itself.
 */
import { ChildProcess, spawn as nodeSpawn, SpawnOptions } from 'child_process';
import { createHash } from 'crypto';
import * as fs from 'fs';
import * as path from 'path';
import { CaseError, checkEnvHosts, loadEnvFile, resolveRepoDir, resolveSpecs, within } from '../caseFs';
import { ExecutorConfig } from '../config';
import { EmitEvent } from '../queue';
import { redact, redactLine, secretValues } from '../redact';
import { CaseRunParams, RunEventLevel } from '../types';

export const MAX_REPORT_BYTES = 2 * 1024 * 1024;
export const TAIL_BYTES = 4096;

export type SpawnFn = (cmd: string, args: string[], opts: SpawnOptions) => ChildProcess;

export interface CaseRunDeps {
  spawn: SpawnFn;
  /** Signals the whole process group of `pid`. */
  killGroup: (pid: number, sig: NodeJS.Signals) => void;
  /** Environment the minimal child env is taken from. */
  baseEnv: NodeJS.ProcessEnv;
  /** Time between SIGTERM and SIGKILL, and the cap on waiting for a dead child. */
  graceMs: number;
}

export const defaultDeps: CaseRunDeps = {
  spawn: nodeSpawn as unknown as SpawnFn,
  killGroup: (pid, sig) => {
    try {
      process.kill(-pid, sig);
    } catch {
      /* already gone */
    }
  },
  baseEnv: process.env,
  graceMs: 3000,
};

const PASSTHROUGH = ['PATH', 'HOME', 'USER', 'LANG', 'LC_ALL', 'TZ', 'TMPDIR', 'PLAYWRIGHT_BROWSERS_PATH', 'XDG_RUNTIME_DIR'];

function minimalEnv(base: NodeJS.ProcessEnv): Record<string, string> {
  const out: Record<string, string> = { CI: '1', NO_COLOR: '1' };
  for (const k of PASSTHROUGH) if (base[k] !== undefined) out[k] = base[k] as string;
  return out;
}

export interface CaseRunContext {
  cfg: ExecutorConfig;
  signal: AbortSignal;
  runId: string;
  emit: EmitEvent;
}

interface ProcResult {
  code: number | null;
  /** Raw (unredacted) tail of stdout. */
  tail: string;
}

function runProc(
  deps: CaseRunDeps,
  ctx: CaseRunContext,
  step: string,
  cmd: string,
  args: string[],
  opts: { cwd: string; env: Record<string, string>; secrets: string[] },
): Promise<ProcResult> {
  return new Promise((resolve, reject) => {
    const { signal } = ctx;
    if (signal.aborted) return reject(signal.reason);
    const child = deps.spawn(cmd, args, { cwd: opts.cwd, env: opts.env, detached: true, shell: false, stdio: ['ignore', 'pipe', 'pipe'] });
    let tail = '';
    let done = false;
    let killTimer: NodeJS.Timeout | undefined;
    let hardTimer: NodeJS.Timeout | undefined;

    const lineReader = (level: RunEventLevel, keepTail: boolean) => {
      let buf = '';
      const push = (line: string) => {
        if (line.trim() === '') return;
        ctx.emit(level, step, redactLine(line, opts.secrets));
      };
      return {
        data: (chunk: Buffer | string) => {
          const s = chunk.toString();
          if (keepTail) tail = (tail + s).slice(-2 * TAIL_BYTES);
          buf += s;
          const parts = buf.split(/\r?\n/);
          buf = parts.pop() ?? '';
          // A pathological line without newline must not grow without bound.
          if (buf.length > 64 * 1024) {
            push(buf);
            buf = '';
          }
          parts.forEach(push);
        },
        flush: () => {
          if (buf) push(buf);
          buf = '';
        },
      };
    };
    const out = lineReader('info', true);
    const err = lineReader('warn', false);
    child.stdout?.on('data', out.data);
    child.stderr?.on('data', err.data);

    const finish = (fn: () => void) => {
      if (done) return;
      done = true;
      clearTimeout(killTimer);
      clearTimeout(hardTimer);
      signal.removeEventListener('abort', onAbort);
      out.flush();
      err.flush();
      fn();
    };
    const onAbort = () => {
      if (child.pid) deps.killGroup(child.pid, 'SIGTERM');
      killTimer = setTimeout(() => child.pid && deps.killGroup(child.pid, 'SIGKILL'), deps.graceMs);
      // Do not wait forever for a child that ignores SIGKILL's aftermath.
      hardTimer = setTimeout(() => finish(() => reject(signal.reason)), deps.graceMs * 2);
    };
    signal.addEventListener('abort', onAbort, { once: true });

    child.on('error', (e) => finish(() => reject(new Error(`failed to start ${cmd}: ${e.message}`))));
    child.on('close', (code) =>
      finish(() => (signal.aborted ? reject(signal.reason) : resolve({ code, tail }))),
    );
  });
}

function lockHash(file: string): string {
  return createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

/** Maps spec paths to Playwright project globs: web -> web-*, api/rest -> api-rest. */
export function projectsFor(specs: string[]): string[] {
  const p = new Set<string>();
  for (const s of specs) p.add(s.startsWith('tests/web/') ? 'web-*' : 'api-rest');
  return [...p];
}

function readReport(repo: string, runId: string): { report: unknown; error?: string } {
  const runsDir = path.join(repo, '.runs');
  const file = path.join(runsDir, runId, 'results.json');
  let real: string;
  try {
    real = fs.realpathSync(file);
  } catch {
    return { report: null, error: 'results.json not found' };
  }
  if (!within(fs.realpathSync(repo), real)) return { report: null, error: 'results.json escapes the repo' };
  const st = fs.statSync(real);
  if (!st.isFile()) return { report: null, error: 'results.json is not a file' };
  if (st.size > MAX_REPORT_BYTES) return { report: null, error: `results.json exceeds ${MAX_REPORT_BYTES} bytes` };
  try {
    return { report: JSON.parse(fs.readFileSync(real, 'utf8')) };
  } catch {
    return { report: null, error: 'results.json is not valid JSON' };
  }
}

function counts(report: unknown, exit: number | null): { passed: number; failed: number; skipped: number } {
  const st = (report as { stats?: Record<string, unknown> } | null)?.stats;
  const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
  if (st && typeof st === 'object') {
    return { passed: num(st.expected) + num(st.flaky), failed: num(st.unexpected), skipped: num(st.skipped) };
  }
  return { passed: 0, failed: exit === 0 ? 0 : 1, skipped: 0 };
}

export async function caseRun(
  ctx: CaseRunContext,
  params: CaseRunParams,
  deps: CaseRunDeps = defaultDeps,
): Promise<Record<string, unknown>> {
  const { cfg, emit } = ctx;
  let repo: string;
  let envVars: Record<string, string>;
  try {
    // Re-check at run time: the tree may have changed since POST.
    repo = resolveRepoDir(params.repo_dir, cfg.repoPool);
    resolveSpecs(repo, params.spec_paths);
    envVars = loadEnvFile(cfg.envDir, params.env_file);
    checkEnvHosts(envVars, cfg.allowedHosts);
  } catch (e) {
    if (e instanceof CaseError) emit('error', 'validate', e.message);
    throw e;
  }
  const secrets = secretValues(envVars);
  emit('info', 'validate', `case ${params.case_code}${params.repair !== undefined ? ` (repair attempt ${params.repair})` : ''}: ${params.spec_paths.length} spec(s), env ${params.env_file}`);

  // npm ci only when the lockfile changed since the last install.
  const lock = path.join(repo, 'package-lock.json');
  const stamp = path.join(repo, 'node_modules', '.qa-lock');
  if (fs.existsSync(lock)) {
    const want = lockHash(lock);
    let have = '';
    try {
      have = fs.readFileSync(stamp, 'utf8').trim();
    } catch {
      /* none yet */
    }
    if (have !== want) {
      emit('info', 'install', 'package-lock.json changed: npm ci');
      const r = await runProc(deps, ctx, 'install', 'npm', ['ci'], { cwd: repo, env: minimalEnv(deps.baseEnv), secrets });
      if (r.code !== 0) {
        emit('error', 'install', `npm ci failed (exit ${r.code})`);
        throw new Error(`npm ci failed (exit ${r.code})`);
      }
      fs.mkdirSync(path.dirname(stamp), { recursive: true });
      fs.writeFileSync(stamp, want);
    } else {
      emit('info', 'install', 'dependencies up to date');
    }
  }

  const runDir = path.join(repo, '.runs', ctx.runId);
  fs.mkdirSync(runDir, { recursive: true });
  const args = [
    'playwright',
    'test',
    ...params.spec_paths,
    ...projectsFor(params.spec_paths).map((p) => `--project=${p}`),
    '-g',
    params.case_code,
    '--workers=1',
    '--retries=0',
    '--reporter=json,list',
  ];
  const env = {
    ...minimalEnv(deps.baseEnv),
    ...envVars,
    // Set last: the env file cannot redirect the run.
    PLAYWRIGHT_JSON_OUTPUT_NAME: path.join(runDir, 'results.json'),
    VENDOR_E2E_RUN: ctx.runId,
    TEST_ENV: 'staging',
  };
  emit('info', 'run', `npx playwright test ${params.spec_paths.join(' ')} -g ${params.case_code}`);
  const r = await runProc(deps, ctx, 'run', 'npx', args, { cwd: repo, env, secrets });

  const { report, error: reportError } = readReport(repo, ctx.runId);
  emit(reportError ? 'warn' : 'info', 'report', reportError ?? 'results.json read');
  const tail = redact(r.tail, secrets).slice(-TAIL_BYTES);
  emit(r.code === 0 ? 'info' : 'warn', 'done', `exit ${r.code}`);
  return {
    exit_code: r.code,
    ...counts(report, r.code),
    report,
    ...(reportError ? { report_error: reportError } : {}),
    tail,
  };
}
