import assert from 'node:assert/strict';
import { ChildProcess } from 'node:child_process';
import { EventEmitter } from 'node:events';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { PassThrough } from 'node:stream';
import { test } from 'node:test';
import { checkEnvHosts, parseEnvFile } from '../src/caseFs';
import { ExecutorConfig } from '../src/config';
import { RunQueue } from '../src/queue';
import { redact, redactLine } from '../src/redact';
import { caseRun, CaseRunDeps, projectsFor, runKey } from '../src/runs/caseRun';
import { CaseRunParams, RunRecord } from '../src/types';
import { parseRunRequest } from '../src/validate';

interface Fx {
  root: string;
  pool: string;
  repo: string;
  envDir: string;
  cfg: ExecutorConfig;
}

function fixture(envBody = 'BASE_URL=https://staging.example.test\nAPI_TOKEN=supersecretvalue\n'): Fx {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'case-run-')));
  const pool = path.join(root, 'pool');
  const repo = path.join(pool, 'automation');
  const envDir = path.join(root, 'env');
  fs.mkdirSync(path.join(repo, 'tests/web'), { recursive: true });
  fs.mkdirSync(path.join(repo, 'tests/api/rest'), { recursive: true });
  fs.mkdirSync(path.join(repo, 'tests/api/mq'), { recursive: true });
  fs.mkdirSync(envDir);
  fs.writeFileSync(path.join(repo, 'playwright.config.ts'), '');
  fs.writeFileSync(path.join(repo, 'package-lock.json'), '{"lock":1}');
  fs.writeFileSync(path.join(repo, 'tests/web/a.spec.ts'), '');
  fs.writeFileSync(path.join(repo, 'tests/api/rest/b.spec.ts'), '');
  fs.writeFileSync(path.join(repo, 'tests/api/mq/c.spec.ts'), '');
  fs.writeFileSync(path.join(envDir, 'automation-1.env'), envBody, { mode: 0o600 });
  fs.chmodSync(path.join(envDir, 'automation-1.env'), 0o600);
  fs.writeFileSync(path.join(envDir, 'playwright-executor.env'), 'EXECUTOR_TOKEN=x', { mode: 0o600 });
  const cfg = {
    repoPool: pool,
    envDir,
    allowedHosts: ['staging.example.test'],
  } as ExecutorConfig;
  return { root, pool, repo, envDir, cfg };
}

const good = (fx: Fx): Record<string, unknown> => ({
  repo_dir: fx.repo,
  case_code: 'TC-REG-001',
  spec_paths: ['tests/web/a.spec.ts'],
  env_file: 'automation-1.env',
});
const parse = (fx: Fx, params: unknown) =>
  parseRunRequest({ type: 'case_run', job_id: 'j', params }, { repoPool: fx.pool, envDir: fx.envDir });

test('case_run validation accepts a good request and returns strict params', () => {
  const fx = fixture();
  const r = parse(fx, { ...good(fx), repair: 2 });
  assert.ok(r.ok);
  assert.deepEqual(r.ok && r.value.params, { ...good(fx), repair: 2 });
});

test('case_run validation rejects every bad param', () => {
  const fx = fixture();
  const g = good(fx);
  const bad: Array<[string, Record<string, unknown>]> = [
    ['unknown key', { ...g, extra: 1 }],
    ['relative repo', { ...g, repo_dir: 'automation' }],
    ['repo outside pool', { ...g, repo_dir: fx.root }],
    ['pool itself', { ...g, repo_dir: fx.pool }],
    ['case code', { ...g, case_code: 'tc-reg-001' }],
    ['case code short', { ...g, case_code: 'TC-REG-01' }],
    ['no specs', { ...g, spec_paths: [] }],
    ['6 specs', { ...g, spec_paths: Array(6).fill('tests/web/a.spec.ts') }],
    ['dotdot', { ...g, spec_paths: ['tests/web/../../x.spec.ts'] }],
    ['absolute spec', { ...g, spec_paths: [path.join(fx.repo, 'tests/web/a.spec.ts')] }],
    ['wrong ext', { ...g, spec_paths: ['tests/web/a.ts'] }],
    ['wrong root', { ...g, spec_paths: ['src/a.spec.ts'] }],
    ['missing spec', { ...g, spec_paths: ['tests/web/none.spec.ts'] }],
    ['mq spec', { ...g, spec_paths: ['tests/api/mq/c.spec.ts'] }],
    ['env path', { ...g, env_file: `${fx.envDir}/automation-1.env` }],
    ['env traversal', { ...g, env_file: '../automation-1.env' }],
    ['env unknown', { ...g, env_file: 'automation-9.env' }],
    ['env executor own', { ...g, env_file: 'playwright-executor.env' }],
    ['repair float', { ...g, repair: 1.5 }],
    ['repair negative', { ...g, repair: -1 }],
    ['repair string', { ...g, repair: '1' }],
  ];
  for (const [name, p] of bad) assert.equal(parse(fx, p).ok, false, name);
  assert.equal(parseRunRequest({ type: 'case_run', job_id: 'j' }, { repoPool: fx.pool, envDir: fx.envDir }).ok, false);
  assert.equal(parseRunRequest({ type: 'case_run', params: good(fx) }).ok, false, 'no case env configured');
});

test('confinement: symlinked repo, spec and config cannot escape the pool', () => {
  const fx = fixture();
  const outside = path.join(fx.root, 'outside');
  fs.mkdirSync(path.join(outside, 'tests/web'), { recursive: true });
  fs.writeFileSync(path.join(outside, 'playwright.config.ts'), '');
  fs.writeFileSync(path.join(outside, 'tests/web/x.spec.ts'), '');
  // repo_dir symlink pointing out of the pool
  fs.symlinkSync(outside, path.join(fx.pool, 'evil'));
  const r1 = parse(fx, { ...good(fx), repo_dir: path.join(fx.pool, 'evil'), spec_paths: ['tests/web/x.spec.ts'] });
  assert.ok(!r1.ok && /outside the repo pool/.test(r1.error));
  // spec symlink pointing out of the repo
  fs.symlinkSync(path.join(outside, 'tests/web/x.spec.ts'), path.join(fx.repo, 'tests/web/link.spec.ts'));
  const r2 = parse(fx, { ...good(fx), spec_paths: ['tests/web/link.spec.ts'] });
  assert.ok(!r2.ok && /escapes the repo/.test(r2.error));
  // directory symlink inside the repo
  fs.symlinkSync(outside, path.join(fx.repo, 'tests/web/dir'));
  assert.equal(parse(fx, { ...good(fx), spec_paths: ['tests/web/dir/x.spec.ts'] }).ok, false);
  // symlink that stays inside is fine
  fs.symlinkSync(path.join(fx.repo, 'tests/web/a.spec.ts'), path.join(fx.repo, 'tests/web/ok.spec.ts'));
  assert.equal(parse(fx, { ...good(fx), spec_paths: ['tests/web/ok.spec.ts'] }).ok, true);
  // symlink inside the repo aiming at an mq spec is refused after resolution
  fs.symlinkSync(path.join(fx.repo, 'tests/api/mq/c.spec.ts'), path.join(fx.repo, 'tests/web/mq.spec.ts'));
  assert.equal(parse(fx, { ...good(fx), spec_paths: ['tests/web/mq.spec.ts'] }).ok, false);
  // env file symlink is not selectable
  fs.symlinkSync(path.join(fx.envDir, 'automation-1.env'), path.join(fx.envDir, 'automation-2.env'));
  assert.equal(parse(fx, { ...good(fx), env_file: 'automation-2.env' }).ok, false);
});

test('redaction masks env values, tokens, bearer and URL userinfo; clips to 300', () => {
  const secrets = ['supersecretvalue'];
  assert.equal(redact('key=supersecretvalue!', secrets), 'key=***!');
  assert.equal(redact('Authorization: Bearer abc.def-123', []), 'Authorization: Bearer ***');
  assert.equal(redact('tok ghp_abcdefghijklmnopqrst1234 end', []), 'tok *** end');
  assert.equal(redact('github_pat_11AAAAAAA_bbbbbbbbbb', []), '***');
  assert.equal(redact('x gaos_abcDEF123456 y', []), 'x *** y');
  assert.equal(redact('git clone https://user:p4ss@host.test/r.git', []), 'git clone https://***@host.test/r.git');
  assert.equal(redact('\u001b[31mred\u001b[0m', []), 'red');
  const long = redactLine(`${'a'.repeat(290)}supersecretvalue`, secrets);
  assert.ok(long.length <= 300);
  assert.ok(!long.includes('supersec'));
  assert.equal(redactLine('x'.repeat(1000), []).length, 300);
});

test('env file parsing and host allow-list', () => {
  assert.deepEqual(parseEnvFile('# c\nA=1\nexport B="two words"\nC=\'x\'\nD=v # note\nbad line\n'), { A: '1', B: 'two words', C: 'x', D: 'v' });
  const allowed = ['staging.example.test', '*.stg.test'];
  checkEnvHosts({ A: 'https://staging.example.test/x', B: 'https://a.stg.test', C: 'plain' }, allowed);
  assert.throws(() => checkEnvHosts({ A: 'https://prod.example.test' }, allowed), /allow-list/);
  assert.throws(() => checkEnvHosts({ A: 'https://stg.test' }, allowed), /allow-list/);
  assert.throws(() => checkEnvHosts({ A: 'https://staging.example.test', B: 'http://evil.test' }, allowed), /evil.test/);
  assert.throws(() => checkEnvHosts({ A: 'nourl' }, allowed), /no http/);
});

test('env file parsing: quoted multi-line and \\n-escaped PEM values', () => {
  const pem = '-----BEGIN PUBLIC KEY-----\nAAAA\nBBBB\n-----END PUBLIC KEY-----';
  const one = parseEnvFile('K="-----BEGIN PUBLIC KEY-----\\nAAAA\\nBBBB\\n-----END PUBLIC KEY-----"\nNEXT=1\n');
  assert.equal(one.K, pem);
  assert.equal(one.NEXT, '1');
  const multi = parseEnvFile('K="-----BEGIN PUBLIC KEY-----\nAAAA\nBBBB\n-----END PUBLIC KEY-----"\nNEXT=2\n');
  assert.equal(multi.K, pem);
  assert.equal(multi.NEXT, '2');
  assert.equal(parseEnvFile("S='a\\nb'\n").S, 'a\\nb', 'single quotes stay literal');
});

test('projectsFor maps spec roots to Playwright projects', () => {
  assert.deepEqual(projectsFor(['tests/web/a.spec.ts']), ['web-*']);
  assert.deepEqual(projectsFor(['tests/api/rest/b.spec.ts']), ['api-rest']);
  assert.deepEqual(projectsFor(['tests/api/graphql/svc/d.spec.ts']), ['api-graphql']);
  assert.deepEqual(projectsFor(['tests/web/a.spec.ts', 'tests/api/rest/b.spec.ts']), ['web-*', 'api-rest']);
  assert.deepEqual(projectsFor(['tests/api/rest/b.spec.ts', 'tests/api/graphql/svc/d.spec.ts']), ['api-rest', 'api-graphql']);
});

test('events ring keeps the last 500 and ?since filters by n', async () => {
  const q = new RunQueue(
    async (_run, _sig, emit) => {
      for (let i = 1; i <= 600; i++) emit('info', 'x', `m${i}`);
      return {};
    },
    { maxQueue: 1, timeoutMs: 5000, ttlMs: 60_000 },
  );
  const run = q.submit({ job_id: 'j', type: 'case_run', params: { repo_dir: '/x', case_code: 'TC-A-001', spec_paths: ['tests/web/a.spec.ts'], env_file: 'automation-1.env' } })!;
  const end = Date.now() + 2000;
  while (q.get(run.id)!.status !== 'completed' && Date.now() < end) await new Promise((r) => setTimeout(r, 5));
  const all = q.get(run.id)!.events;
  assert.equal(all.length, 500);
  assert.equal(all[0].n, 101);
  assert.equal(all[499].n, 600);
  assert.equal(all[499].message, 'm600');
  const since = q.get(run.id, 590)!.events;
  assert.deepEqual(since.map((e) => e.n), [591, 592, 593, 594, 595, 596, 597, 598, 599, 600]);
  assert.equal(q.get(run.id, 600)!.events.length, 0);
  assert.equal(q.get(run.id)!.events.length, 500, 'since does not mutate the record');
  assert.equal(q.get('nope', 1), undefined);
});

interface Call {
  cmd: string;
  args: string[];
  opts: Record<string, any>;
}

/** Fake child: `script` writes output and returns the exit code. */
function fakeDeps(fx: Fx, script: (call: Call, c: { out: PassThrough; err: PassThrough }) => number | 'hang', calls: Call[], kills: Array<[number, string]> = []): CaseRunDeps {
  let pid = 1000;
  return {
    spawn: ((cmd: string, args: string[], opts: Record<string, any>) => {
      const call = { cmd, args, opts };
      calls.push(call);
      const child = new EventEmitter() as EventEmitter & { stdout: PassThrough; stderr: PassThrough; pid: number };
      child.stdout = new PassThrough();
      child.stderr = new PassThrough();
      child.pid = ++pid;
      setImmediate(() => {
        // The browser install is infrastructure, not the scripted test run.
        const isInstall = cmd === 'npx' && args[1] === 'install';
        const code = isInstall ? 0 : script(call, { out: child.stdout, err: child.stderr });
        if (code === 'hang') return;
        setImmediate(() => child.emit('close', code));
      });
      return child as unknown as ChildProcess;
    }) as unknown as CaseRunDeps['spawn'],
    killGroup: (p, s) => kills.push([p, s]),
    baseEnv: { PATH: '/bin', HOME: '/home/x', EXECUTOR_TOKEN: 'leak-me', TARGET_CREDENTIALS_FILE: '/secret', SECRET_OTHER: 'nope' },
    graceMs: 20,
  };
}

const params = (fx: Fx): CaseRunParams => good(fx) as unknown as CaseRunParams;

test('caseRun: npm ci then playwright, minimal env, redacted events, report and counts', async () => {
  const fx = fixture();
  const calls: Call[] = [];
  const events: Array<[string, string, string]> = [];
  const report = { stats: { expected: 1, unexpected: 0, skipped: 0, flaky: 0 }, suites: [] };
  const deps = fakeDeps(
    fx,
    (call, c) => {
      if (call.cmd === 'npm') {
        c.out.write('added 5 packages\n');
        return 0;
      }
      c.out.write('start API_TOKEN=supersecretvalue Bearer zzz.yyy\n');
      c.out.write(`${'z'.repeat(500)}\n`);
      c.err.write('warn line\n');
      const runDir = path.join(fx.repo, '.runs', call.opts.env.VENDOR_E2E_RUN);
      fs.mkdirSync(runDir, { recursive: true });
      fs.writeFileSync(path.join(runDir, 'results.json'), JSON.stringify(report));
      c.out.write('last supersecretvalue');
      return 0;
    },
    calls,
  );
  const res = await caseRun({ cfg: fx.cfg, signal: new AbortController().signal, runId: 'run-1', emit: (l, s, m) => events.push([l, s, m]) }, params(fx), deps);

  assert.deepEqual(calls.map((c) => [c.cmd, c.args[0]]), [['npm', 'ci'], ['npx', 'playwright'], ['npx', 'playwright']]);
  assert.deepEqual(calls[1].args, ['playwright', 'install', 'chromium'], 'web specs install the repo\'s browser build');
  const pw = calls[2];
  assert.deepEqual(pw.args, ['playwright', 'test', 'tests/web/a.spec.ts', '--project=web-*', '-g', 'TC-REG-001', '--workers=1', '--retries=0', '--reporter=json,list']);
  for (const c of calls) {
    assert.equal(c.opts.shell, false);
    assert.equal(c.opts.detached, true);
    assert.equal(c.opts.cwd, fx.repo);
    assert.equal(c.opts.env.EXECUTOR_TOKEN, undefined);
    assert.equal(c.opts.env.TARGET_CREDENTIALS_FILE, undefined);
    assert.equal(c.opts.env.SECRET_OTHER, undefined);
    assert.equal(c.opts.env.PATH, '/bin');
  }
  assert.equal(calls[0].opts.env.API_TOKEN, undefined, 'npm ci gets no env-file keys');
  assert.equal(pw.opts.env.API_TOKEN, 'supersecretvalue');
  assert.equal(pw.opts.env.VENDOR_E2E_RUN, runKey('run-1'));
  assert.match(pw.opts.env.VENDOR_E2E_RUN, /^[A-Za-z0-9]{3,24}$/);
  assert.equal(pw.opts.env.TEST_ENV, 'staging');
  assert.ok(fs.readFileSync(path.join(fx.repo, 'node_modules/.qa-lock'), 'utf8').length === 64);

  const msgs = events.map((e) => e[2]);
  assert.ok(msgs.includes('start API_TOKEN=*** Bearer ***'));
  assert.ok(events.every((e) => e[2].length <= 300));
  assert.ok(events.some((e) => e[0] === 'warn' && e[2] === 'warn line'));
  assert.ok(!JSON.stringify(events).includes('supersecretvalue'));

  assert.equal(res.exit_code, 0);
  assert.deepEqual([res.passed, res.failed, res.skipped], [1, 0, 0]);
  assert.deepEqual(res.report, report);
  assert.ok(!(res.tail as string).includes('supersecretvalue'));
  assert.ok((res.tail as string).endsWith('last ***'));
  assert.ok((res.tail as string).length <= 4096);
});

test('caseRun: second run skips npm ci when the lockfile hash matches; failures give counts', async () => {
  const fx = fixture();
  fs.mkdirSync(path.join(fx.repo, 'node_modules'));
  const { createHash } = await import('node:crypto');
  fs.writeFileSync(path.join(fx.repo, 'node_modules/.qa-lock'), createHash('sha256').update(fs.readFileSync(path.join(fx.repo, 'package-lock.json'))).digest('hex'));
  fs.copyFileSync(path.join(fx.repo, 'node_modules/.qa-lock'), path.join(fx.repo, 'node_modules/.qa-browsers'));
  const calls: Call[] = [];
  const deps = fakeDeps(fx, (call, c) => {
    c.out.write('1 failed\n');
    const runDir = path.join(fx.repo, '.runs', call.opts.env.VENDOR_E2E_RUN);
    fs.mkdirSync(runDir, { recursive: true });
    fs.writeFileSync(path.join(runDir, 'results.json'), JSON.stringify({ stats: { expected: 0, unexpected: 1, skipped: 2 } }));
    return 1;
  }, calls);
  const res = await caseRun({ cfg: fx.cfg, signal: new AbortController().signal, runId: 'run-2', emit: () => undefined }, params(fx), deps);
  assert.deepEqual(calls.map((c) => c.cmd), ['npx']);
  assert.equal(res.exit_code, 1);
  assert.deepEqual([res.passed, res.failed, res.skipped], [0, 1, 2]);
});

test('caseRun: oversized or missing report yields null report with report_error', async () => {
  const fx = fixture();
  const deps = fakeDeps(fx, (call) => {
    if (call.cmd !== 'npx') return 0;
    const runDir = path.join(fx.repo, '.runs', call.opts.env.VENDOR_E2E_RUN);
    fs.mkdirSync(runDir, { recursive: true });
    fs.writeFileSync(path.join(runDir, 'results.json'), Buffer.alloc(2 * 1024 * 1024 + 1, 'a'));
    return 0;
  }, []);
  const res = await caseRun({ cfg: fx.cfg, signal: new AbortController().signal, runId: 'run-3', emit: () => undefined }, params(fx), deps);
  assert.equal(res.report, null);
  assert.match(res.report_error as string, /exceeds/);
});

test('caseRun refuses non-allow-listed hosts, loose env mode, and never spawns', async () => {
  const fx = fixture('BASE_URL=https://prod.example.test\n');
  const calls: Call[] = [];
  const deps = fakeDeps(fx, () => 0, calls);
  const ctx = { cfg: fx.cfg, signal: new AbortController().signal, runId: 'r', emit: () => undefined };
  await assert.rejects(caseRun(ctx, params(fx), deps), /allow-list/);
  fs.writeFileSync(path.join(fx.envDir, 'automation-1.env'), 'BASE_URL=https://staging.example.test\n');
  fs.chmodSync(path.join(fx.envDir, 'automation-1.env'), 0o644);
  await assert.rejects(caseRun(ctx, params(fx), deps), /mode 600/);
  assert.equal(calls.length, 0);
});

test('caseRun: abort kills the process group (TERM then KILL) and rejects', async () => {
  const fx = fixture();
  const kills: Array<[number, string]> = [];
  const deps = fakeDeps(fx, () => 'hang', [], kills);
  fs.mkdirSync(path.join(fx.repo, 'node_modules'));
  const { createHash } = await import('node:crypto');
  fs.writeFileSync(path.join(fx.repo, 'node_modules/.qa-lock'), createHash('sha256').update(fs.readFileSync(path.join(fx.repo, 'package-lock.json'))).digest('hex'));
  fs.copyFileSync(path.join(fx.repo, 'node_modules/.qa-lock'), path.join(fx.repo, 'node_modules/.qa-browsers'));
  const ctrl = new AbortController();
  const p = caseRun({ cfg: fx.cfg, signal: ctrl.signal, runId: 'r', emit: () => undefined }, params(fx), deps);
  setTimeout(() => ctrl.abort(new Error('run exceeded 1s timeout')), 30);
  await assert.rejects(p, /timeout/);
  assert.deepEqual(kills.map((k) => k[1]), ['SIGTERM', 'SIGKILL']);
  assert.ok(kills.every((k) => k[0] === 1001));
});

test('case_run through the queue records events and result', async () => {
  const fx = fixture();
  const deps = fakeDeps(fx, (call, c) => {
    c.out.write('hello\n');
    return 0;
  }, []);
  const q = new RunQueue((run: RunRecord, signal, emit) => caseRun({ cfg: fx.cfg, signal, runId: run.id, emit }, run.params as CaseRunParams, deps), {
    maxQueue: 1,
    timeoutMs: 5000,
    ttlMs: 60_000,
  });
  const parsed = parse(fx, good(fx));
  assert.ok(parsed.ok);
  const run = q.submit(parsed.value)!;
  const end = Date.now() + 3000;
  while (q.get(run.id)!.status === 'queued' || q.get(run.id)!.status === 'running') {
    if (Date.now() > end) throw new Error('timeout');
    await new Promise((r) => setTimeout(r, 5));
  }
  const rec = q.get(run.id)!;
  assert.equal(rec.status, 'completed');
  assert.ok(rec.events.some((e) => e.step === 'run' && e.message === 'hello'));
  assert.equal((rec.result as { exit_code: number }).exit_code, 0);
});

test('caseRun: each instance reports to its own Brain Town and masks only its own token', async () => {
  const fx = fixture();
  fs.mkdirSync(path.join(fx.repo, 'node_modules'));
  const { createHash } = await import('node:crypto');
  fs.writeFileSync(path.join(fx.repo, 'package-lock.json'), '{}');
  const stamp = createHash('sha256').update(fs.readFileSync(path.join(fx.repo, 'package-lock.json'))).digest('hex');
  fs.writeFileSync(path.join(fx.repo, 'node_modules/.qa-lock'), stamp);
  fs.writeFileSync(path.join(fx.repo, 'node_modules/.qa-browsers'), stamp);
  const mk = (url: string, token: string, org: string) => ({ reporter: '/r/brain-town-reporter.cjs', url, token, org, product: 'p', products: {} });
  const cfg = {
    ...fx.cfg,
    brainTownFor: (i: string) => (i === 'personal' ? mk('http://bt/personal', 'bt_live_personal', 'ardihikaru') : i === '' ? mk('http://bt/default', 'bt_live_default', 'work') : { ...mk('', '', 'x'), reporter: '' }),
  } as ExecutorConfig;
  const run = async (instance: string | undefined) => {
    const calls: Call[] = [];
    const events: string[] = [];
    const deps = fakeDeps(fx, (call, c) => {
      if (call.cmd === 'npx') c.out.write('tokens bt_live_personal bt_live_default\n');
      return 0;
    }, calls);
    await caseRun({ cfg, signal: new AbortController().signal, runId: 'r', emit: (_l, _s, m) => events.push(m), instance }, params(fx), deps);
    const pw = calls.find((c) => c.cmd === 'npx' && c.args[0] === 'playwright' && c.args[1] === 'test')!;
    return { env: pw.opts.env, args: pw.args, events };
  };
  const personal = await run('personal');
  assert.deepEqual([personal.env.BRAIN_TOWN_URL, personal.env.BRAIN_TOWN_TOKEN, personal.env.BRAIN_TOWN_ORG], ['http://bt/personal', 'bt_live_personal', 'ardihikaru']);
  assert.ok(personal.args.some((a) => a.includes('/r/brain-town-reporter.cjs')));
  assert.ok(!personal.events.join('\n').includes('bt_live_personal'));
  const dflt = await run(undefined);
  assert.deepEqual([dflt.env.BRAIN_TOWN_URL, dflt.env.BRAIN_TOWN_TOKEN, dflt.env.BRAIN_TOWN_ORG], ['http://bt/default', 'bt_live_default', 'work']);
  // An instance with no Brain Town config: reporter not added, no BRAIN_TOWN_* in the child env.
  const none = await run('other');
  assert.equal(none.env.BRAIN_TOWN_URL, undefined);
  assert.ok(!none.args.some((a) => a.includes('brain-town-reporter')));
});
