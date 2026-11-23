import assert from 'node:assert/strict';
import * as fs from 'node:fs';
import { AddressInfo } from 'node:net';
import * as os from 'node:os';
import * as path from 'node:path';
import { test } from 'node:test';
import { checkBearer } from '../src/auth';
import { ExecutorConfig, loadConfig } from '../src/config';
import { RunQueue } from '../src/queue';
import { createServer } from '../src/server';
import { RunRecord } from '../src/types';
import { parseRunRequest, RunRequest } from '../src/validate';

const TOKEN = 'x'.repeat(40);
const EXTRA = 'y'.repeat(40);
const req = (over: Partial<RunRequest> = {}): RunRequest => ({
  job_id: 'job-1',
  type: 'case_run',
  params: { repo_dir: '/x', case_code: 'TC-A-001', spec_paths: ['tests/web/a.spec.ts'], env_file: 'automation-1.env' },
  ...over,
});
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
async function waitFor(fn: () => boolean, ms = 2000) {
  const end = Date.now() + ms;
  while (!fn()) {
    if (Date.now() > end) throw new Error('timeout');
    await sleep(5);
  }
}

test('checkBearer accepts only the exact token', () => {
  assert.equal(checkBearer(`Bearer ${TOKEN}`, TOKEN), true);
  assert.equal(checkBearer(`bearer ${TOKEN}`, TOKEN), true);
  assert.equal(checkBearer(`Bearer ${TOKEN}x`, TOKEN), false);
  assert.equal(checkBearer(TOKEN, TOKEN), false);
  assert.equal(checkBearer(undefined, TOKEN), false);
  assert.equal(checkBearer('Bearer ', ''), false);
});

test('parseRunRequest rejects anything but a well-formed case_run', () => {
  for (const bad of [null, [], { type: 'rm -rf' }, { type: 'qa_run' }, { type: 'docs_generate' }, { type: 'case_run', job_id: 'j'.repeat(129) }]) {
    assert.equal(parseRunRequest(bad, { repoPool: '/tmp/unused', envDir: '/tmp/unused' }).ok, false, JSON.stringify(bad));
  }
});

test('RunQueue runs one at a time, in order, and rejects when full', async () => {
  let running = 0;
  let maxRunning = 0;
  const order: string[] = [];
  const release: Array<() => void> = [];
  const q = new RunQueue(
    async (run) => {
      running++;
      maxRunning = Math.max(maxRunning, running);
      order.push(run.job_id);
      await new Promise<void>((r) => release.push(r));
      running--;
      return { ok: true };
    },
    { maxQueue: 2, timeoutMs: 5000, ttlMs: 60_000 },
  );
  const a = q.submit(req({ job_id: 'a' }))!;
  await waitFor(() => q.stats().busy);
  const b = q.submit(req({ job_id: 'b' }))!;
  const c = q.submit(req({ job_id: 'c' }))!;
  assert.equal(q.submit(req({ job_id: 'd' })), null, 'queue should be full');
  assert.equal(q.stats().queued, 2);

  for (let i = 0; i < 3; i++) {
    await waitFor(() => release.length > 0);
    release.shift()!();
  }
  await waitFor(() => [a, b, c].every((r) => q.get(r.id)!.status === 'completed'));
  assert.deepEqual(order, ['a', 'b', 'c']);
  assert.equal(maxRunning, 1);
  assert.ok(q.get(a.id)!.duration_ms! >= 0);
});

test('RunQueue fails a run that exceeds its timeout and keeps going', async () => {
  let closed = false;
  const q = new RunQueue(
    (run: RunRecord, signal: AbortSignal) =>
      new Promise((resolve, reject) => {
        if (run.job_id === 'fast') return resolve({ fast: true });
        signal.addEventListener('abort', () => {
          closed = true; // stands in for browser.close()
          reject(new Error('browser closed'));
        });
      }),
    { maxQueue: 5, timeoutMs: 50, ttlMs: 60_000 },
  );
  const slow = q.submit(req({ job_id: 'slow' }))!;
  const fast = q.submit(req({ job_id: 'fast' }))!;
  await waitFor(() => q.get(fast.id)!.status === 'completed');
  assert.equal(q.get(slow.id)!.status, 'failed');
  assert.match(q.get(slow.id)!.error!, /timeout/);
  assert.equal(closed, true);
});

test('HTTP API: health is public, runs need the bearer token', async () => {
  const root = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'exec-http-')));
  const pool = path.join(root, 'pool');
  const envDir = path.join(root, 'env');
  fs.mkdirSync(path.join(pool, 'auto/tests/web'), { recursive: true });
  fs.mkdirSync(envDir);
  fs.writeFileSync(path.join(pool, 'auto/playwright.config.ts'), '');
  fs.writeFileSync(path.join(pool, 'auto/tests/web/a.spec.ts'), '');
  fs.writeFileSync(path.join(envDir, 'automation-1.env'), 'BASE_URL=https://staging.invalid\n', { mode: 0o600 });
  const caseParams = { repo_dir: path.join(pool, 'auto'), case_code: 'TC-A-001', spec_paths: ['tests/web/a.spec.ts'], env_file: 'automation-1.env' };
  const cfg: ExecutorConfig = {
    bind: '127.0.0.1',
    port: 0,
    token: TOKEN,
    tokens: [TOKEN, EXTRA],
    tokenInstances: { [EXTRA]: 'personal' },
    runTimeoutMs: 1000,
    maxQueue: 1,
    repoPool: pool,
    envDir,
    allowedHosts: ['staging.invalid'],
  };
  const seen: Array<string | undefined> = [];
  const q = new RunQueue(async (run) => (seen.push(run.instance), { done: true }), { maxQueue: 1, timeoutMs: 1000, ttlMs: 60_000 });
  const server = createServer(cfg, q);
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  const auth = { Authorization: `Bearer ${TOKEN}`, 'Content-Type': 'application/json' };
  try {
    const h = await fetch(`${base}/health`);
    assert.ok([200, 503].includes(h.status));
    assert.equal((await h.json()).service, 'playwright-executor');

    assert.equal((await fetch(`${base}/v1/runs`, { method: 'POST', body: '{}' })).status, 401);
    assert.equal((await fetch(`${base}/v1/runs/x`, { headers: { Authorization: 'Bearer nope' } })).status, 401);
    assert.equal((await fetch(`${base}/v1/runs`, { method: 'POST', headers: auth, body: '{"type":"nope"}' })).status, 400);
    assert.equal((await fetch(`${base}/v1/runs`, { method: 'POST', headers: auth, body: 'not json' })).status, 400);

    const created = await fetch(`${base}/v1/runs`, { method: 'POST', headers: auth, body: JSON.stringify({ type: 'case_run', job_id: 'j1', params: caseParams }) });
    assert.equal(created.status, 202);
    const { id } = await created.json();
    let run: RunRecord | undefined;
    for (let i = 0; i < 100; i++) {
      run = await (await fetch(`${base}/v1/runs/${id}`, { headers: auth })).json();
      if (run!.status === 'completed') break;
      await sleep(10);
    }
    assert.equal(run!.status, 'completed');
    assert.deepEqual(run!.result, { done: true });
    assert.equal((await fetch(`${base}/v1/runs/run-missing`, { headers: auth })).status, 404);
    // A second instance's token (EXECUTOR_EXTRA_TOKENS) is accepted too.
    assert.equal((await fetch(`${base}/v1/runs/${id}`, { headers: { Authorization: `Bearer ${EXTRA}` } })).status, 200);
    // The run carries the instance of the token that submitted it (never the token itself).
    assert.equal(run!.instance, '');
    const second = await fetch(`${base}/v1/runs`, { method: 'POST', headers: { ...auth, Authorization: `Bearer ${EXTRA}` }, body: JSON.stringify({ type: 'case_run', job_id: 'j2', params: caseParams }) });
    assert.equal(second.status, 202);
    const { id: id2 } = await second.json();
    let run2: RunRecord | undefined;
    for (let i = 0; i < 100; i++) {
      run2 = await (await fetch(`${base}/v1/runs/${id2}`, { headers: auth })).json();
      if (run2!.status === 'completed') break;
      await sleep(10);
    }
    assert.equal(run2!.instance, 'personal');
    assert.deepEqual(seen, ['', 'personal']);
    assert.ok(!JSON.stringify(run2).includes(EXTRA) && !JSON.stringify(run2).includes(TOKEN));
  } finally {
    server.close();
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test('loadConfig: EXECUTOR_EXTRA_TOKENS adds tokens and rejects short ones', () => {
  const base = { EXECUTOR_TOKEN: TOKEN, HOME: '/home/x' };
  assert.deepEqual(loadConfig(base).tokens, [TOKEN]);
  assert.deepEqual(loadConfig({ ...base, EXECUTOR_EXTRA_TOKENS: ` ${EXTRA} ,, ${TOKEN}` }).tokens, [TOKEN, EXTRA]);
  assert.throws(() => loadConfig({ ...base, EXECUTOR_EXTRA_TOKENS: 'short' }), /EXECUTOR_EXTRA_TOKENS/);
});

test('loadConfig: EXECUTOR_EXTRA_TOKENS accepts name:token and bare tokens', () => {
  const base = { EXECUTOR_TOKEN: TOKEN, HOME: '/home/x' };
  const named = loadConfig({ ...base, EXECUTOR_EXTRA_TOKENS: `Personal:${EXTRA}` });
  assert.deepEqual(named.tokens, [TOKEN, EXTRA]);
  assert.deepEqual(named.tokenInstances, { [EXTRA]: 'personal' });
  // A bare token is the default instance (no mapping).
  assert.deepEqual(loadConfig({ ...base, EXECUTOR_EXTRA_TOKENS: EXTRA }).tokenInstances, {});
  // The token part carries the length check, not the name.
  assert.throws(() => loadConfig({ ...base, EXECUTOR_EXTRA_TOKENS: 'personal:short' }), /EXECUTOR_EXTRA_TOKENS/);
  for (const bad of ['1abc', '-x', 'a b', 'a'.repeat(33), '']) {
    assert.throws(() => loadConfig({ ...base, EXECUTOR_EXTRA_TOKENS: `${bad}:${EXTRA}` }), /instance names/, bad);
  }
  // EXECUTOR_TOKEN stays the default even if repeated under a name.
  const dup = loadConfig({ ...base, EXECUTOR_EXTRA_TOKENS: `personal:${TOKEN}` });
  assert.deepEqual(dup.tokens, [TOKEN]);
  assert.deepEqual(dup.tokenInstances, {});
});

test('loadConfig: Brain Town config is per instance and never falls back to the default', () => {
  const base = {
    EXECUTOR_TOKEN: TOKEN,
    HOME: '/home/x',
    EXECUTOR_EXTRA_TOKENS: `personal:${EXTRA},work-2:${'z'.repeat(40)}`,
    BRAIN_TOWN_URL: 'http://bt/default',
    BRAIN_TOWN_TOKEN: 'bt_live_default',
    BRAIN_TOWN_URL__PERSONAL: 'http://bt/personal',
    BRAIN_TOWN_TOKEN__PERSONAL: 'bt_live_personal',
    BRAIN_TOWN_ORG__PERSONAL: 'ardihikaru',
    BRAIN_TOWN_PRODUCTS__PERSONAL: '1=site',
  };
  const cfg = loadConfig(base);
  assert.equal(cfg.brainTownFor!('').url, 'http://bt/default');
  assert.equal(cfg.brainTownFor!('').token, 'bt_live_default');
  const p = cfg.brainTownFor!('personal');
  assert.deepEqual([p.url, p.token, p.org, p.products], ['http://bt/personal', 'bt_live_personal', 'ardihikaru', { 1: 'site' }]);
  // work-2 has no URL: off, and nothing of the default (or personal) leaks in.
  const w = cfg.brainTownFor!('work-2');
  assert.deepEqual([w.reporter, w.url, w.token], ['', '', '']);
  // Only a default URL: a named instance stays off.
  const only = loadConfig({ EXECUTOR_TOKEN: TOKEN, HOME: '/h', EXECUTOR_EXTRA_TOKENS: `personal:${EXTRA}`, BRAIN_TOWN_URL: 'http://bt/default' });
  assert.equal(only.brainTownFor!('personal').url, '');
  assert.equal(only.brainTownFor!('personal').reporter, '');
  assert.notEqual(only.brainTownFor!('').reporter, '');
  // The kill switch disables every instance.
  const off = loadConfig({ ...base, EXECUTOR_BRAIN_TOWN_REPORTER: '0' });
  assert.equal(off.brainTownFor!('').reporter, '');
  assert.equal(off.brainTownFor!('personal').reporter, '');
});
