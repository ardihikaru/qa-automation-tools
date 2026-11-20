/**
 * The navigation reporter (docs/designs/NAVIGATION_MAP.md 3.5): how case_run wires it, and the
 * reporter itself in a real Playwright run of the executor's pinned version (request steps only,
 * so no browser is needed).
 */
import assert from 'node:assert/strict';
import { ChildProcess, spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import * as fs from 'node:fs';
import * as http from 'node:http';
import * as os from 'node:os';
import * as path from 'node:path';
import { PassThrough } from 'node:stream';
import { test } from 'node:test';
import { ExecutorConfig } from '../src/config';
import { NAV_REPORTER_PATH, resolveNavReporter } from '../src/navReporter';
import { caseRun, CaseRunDeps } from '../src/runs/caseRun';
import { CaseRunParams } from '../src/types';

test('resolveNavReporter: on by default, off by flag, missing or not a reporter is off', () => {
  assert.equal(resolveNavReporter({}), NAV_REPORTER_PATH);
  assert.ok(fs.existsSync(NAV_REPORTER_PATH));
  assert.equal(resolveNavReporter({ EXECUTOR_NAV_REPORTER: '0' }), '');
  assert.equal(resolveNavReporter({}, '/nonexistent/nav-reporter.cjs'), '');
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'nav-rep-'));
  const bad = path.join(dir, 'bad.cjs');
  fs.writeFileSync(bad, 'module.exports = 42;');
  assert.equal(resolveNavReporter({}, bad), '');
  const broken = path.join(dir, 'broken.cjs');
  fs.writeFileSync(broken, 'throw new Error("boom");');
  assert.equal(resolveNavReporter({}, broken), '');
  assert.equal(resolveNavReporter({}, path.join(dir, 'a,b.cjs')), '');
});

function repoFixture(): { repo: string; pool: string; envDir: string } {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'nav-run-')));
  const pool = path.join(root, 'pool');
  const repo = path.join(pool, 'automation');
  const envDir = path.join(root, 'env');
  fs.mkdirSync(path.join(repo, 'tests/api/rest'), { recursive: true });
  fs.mkdirSync(path.join(repo, 'node_modules'), { recursive: true });
  fs.mkdirSync(envDir);
  fs.writeFileSync(path.join(repo, 'playwright.config.ts'), '');
  fs.writeFileSync(path.join(repo, 'package-lock.json'), '{"lock":1}');
  fs.writeFileSync(path.join(repo, 'tests/api/rest/b.spec.ts'), '');
  const { createHash } = require('node:crypto');
  fs.writeFileSync(path.join(repo, 'node_modules/.qa-lock'), createHash('sha256').update(fs.readFileSync(path.join(repo, 'package-lock.json'))).digest('hex'));
  fs.writeFileSync(path.join(envDir, 'automation-1.env'), 'BASE_URL=https://staging.example.test\n', { mode: 0o600 });
  fs.chmodSync(path.join(envDir, 'automation-1.env'), 0o600);
  return { repo, pool, envDir };
}

function fakeSpawn(onRun: (env: Record<string, string>) => void, calls: string[][]): CaseRunDeps {
  return {
    spawn: ((_cmd: string, args: string[], opts: { env: Record<string, string> }) => {
      calls.push(args);
      const child = new EventEmitter() as EventEmitter & { stdout: PassThrough; stderr: PassThrough; pid: number };
      child.stdout = new PassThrough();
      child.stderr = new PassThrough();
      child.pid = 4242;
      setImmediate(() => {
        onRun(opts.env);
        setImmediate(() => child.emit('close', 0));
      });
      return child as unknown as ChildProcess;
    }) as unknown as CaseRunDeps['spawn'],
    killGroup: () => undefined,
    baseEnv: { PATH: '/bin', HOME: '/home/x' },
    graceMs: 20,
  };
}

test('caseRun: adds the reporter and QA_NAV_OUTPUT, returns nav; without it, nothing changes', async () => {
  const fx = repoFixture();
  const params = { repo_dir: fx.repo, case_code: 'TC-REG-001', spec_paths: ['tests/api/rest/b.spec.ts'], env_file: 'automation-1.env' } as unknown as CaseRunParams;
  const nav = { version: 1, truncated: false, entries: [{ title: 'TC-REG-001 x', kind: 'api', method: 'GET', host: 'h', path: '/a', hits: 1 }] };
  const write = (env: Record<string, string>, withNav: boolean) => {
    const runDir = path.join(fx.repo, '.runs', env.VENDOR_E2E_RUN);
    fs.mkdirSync(runDir, { recursive: true });
    fs.writeFileSync(path.join(runDir, 'results.json'), JSON.stringify({ stats: { expected: 1 }, suites: [] }));
    if (withNav && env.QA_NAV_OUTPUT) fs.writeFileSync(env.QA_NAV_OUTPUT, JSON.stringify(nav));
  };

  const calls: string[][] = [];
  let seen: Record<string, string> = {};
  const cfg = { repoPool: fx.pool, envDir: fx.envDir, allowedHosts: ['staging.example.test'], navReporter: NAV_REPORTER_PATH } as ExecutorConfig;
  const res = await caseRun({ cfg, signal: new AbortController().signal, runId: 'run-n1', emit: () => undefined }, params, fakeSpawn((env) => {
    seen = env;
    write(env, true);
  }, calls));
  assert.ok(calls[0].includes(`--reporter=json,list,${NAV_REPORTER_PATH}`));
  assert.equal(seen.QA_NAV_OUTPUT, path.join(fx.repo, '.runs', seen.VENDOR_E2E_RUN, 'nav.json'));
  assert.deepEqual(res.nav, nav);

  // A run that wrote no nav.json returns no nav (a stale one from an earlier run is removed first).
  const events: string[] = [];
  const res2 = await caseRun({ cfg, signal: new AbortController().signal, runId: 'run-n1', emit: (_l, _s, m) => events.push(m) }, params, fakeSpawn((env) => write(env, false), []));
  assert.equal(res2.nav, undefined);
  assert.ok(events.some((m) => m.startsWith('navigation not recorded')));

  // Off: the old arguments, no QA_NAV_OUTPUT, no nav.
  const calls3: string[][] = [];
  let seen3: Record<string, string> = {};
  const off = { ...cfg, navReporter: '' } as ExecutorConfig;
  const res3 = await caseRun({ cfg: off, signal: new AbortController().signal, runId: 'run-n3', emit: () => undefined }, params, fakeSpawn((env) => {
    seen3 = env;
    write(env, true);
  }, calls3));
  assert.ok(calls3[0].includes('--reporter=json,list'));
  assert.equal(seen3.QA_NAV_OUTPUT, undefined);
  assert.equal(res3.nav, undefined);
});

test('nav reporter in a real Playwright run records API requests without the query', async (t) => {
  const cli = path.resolve(__dirname, '..', '..', 'node_modules', 'playwright', 'cli.js');
  if (!fs.existsSync(cli)) {
    t.skip('playwright is not installed');
    return;
  }
  const server = http.createServer((_req, res) => res.end('{}'));
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  const port = (server.address() as { port: number }).port;
  try {
    const dir = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'nav-pw-')));
    fs.mkdirSync(path.join(dir, 'tests/api/rest/svc'), { recursive: true });
    fs.writeFileSync(path.join(dir, 'playwright.config.js'),
      `module.exports = { testDir: './tests', projects: [{ name: 'api-rest', use: { baseURL: 'http://127.0.0.1:${port}/api/v1/' } }] };\n`);
    fs.writeFileSync(path.join(dir, 'tests/api/rest/svc/a.spec.js'), `
const { test } = require(${JSON.stringify(path.resolve(__dirname, '..', '..', 'node_modules', 'playwright', 'test'))});
test('TC-NAV-001 probe', async ({ request }) => {
  await request.get('service-status?token=secret');
  await request.get('service-status');
  await request.post('http://127.0.0.1:${port}/api/v1/kiosk-his-v2/patients', { data: { a: 1 } });
  await request.fetch('swagger/index.html');
});
`);
    const out = path.join(dir, 'nav.json');
    // Async: the test server answers from this process while Playwright runs.
    const r = await new Promise<{ status: number | null; output: string }>((resolve) => {
      const child = spawn(process.execPath, [cli, 'test', `--reporter=${NAV_REPORTER_PATH}`], { cwd: dir, env: { ...process.env, QA_NAV_OUTPUT: out, CI: '1' } });
      let output = '';
      child.stdout.on('data', (b) => (output += b));
      child.stderr.on('data', (b) => (output += b));
      const timer = setTimeout(() => child.kill('SIGKILL'), 60_000);
      child.on('close', (status) => {
        clearTimeout(timer);
        resolve({ status, output });
      });
    });
    assert.equal(r.status, 0, r.output);
    const nav = JSON.parse(fs.readFileSync(out, 'utf8'));
    assert.equal(nav.version, 1);
    assert.equal(nav.truncated, false);
    const got = nav.entries.map((e: Record<string, unknown>) => `${e.kind} ${e.method} ${e.path} ${e.hits} ${e.file} ${e.project}`).sort();
    assert.deepEqual(got, [
      'api GET /api/v1/service-status 2 tests/api/rest/svc/a.spec.js api-rest',
      'api GET /api/v1/swagger/index.html 1 tests/api/rest/svc/a.spec.js api-rest',
      'api POST /api/v1/kiosk-his-v2/patients 1 tests/api/rest/svc/a.spec.js api-rest',
    ]);
    assert.ok(nav.entries.every((e: Record<string, unknown>) => e.title === 'TC-NAV-001 probe' && e.host === `127.0.0.1:${port}`));
    assert.ok(!JSON.stringify(nav).includes('secret'));
  } finally {
    server.close();
  }
});
