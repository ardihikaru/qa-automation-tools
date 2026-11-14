import assert from 'node:assert/strict';
import { AddressInfo } from 'node:net';
import { test } from 'node:test';
import { checkBearer } from '../src/auth';
import { ExecutorConfig, parseCredentials } from '../src/config';
import { RunQueue } from '../src/queue';
import { createServer } from '../src/server';
import { RunParams, RunRecord } from '../src/types';
import { parseRunRequest, RunRequest } from '../src/validate';

const TOKEN = 'x'.repeat(40);
const ROLES = ['admin', 'doctor_gp', 'cashier'];
const req = (over: Partial<RunRequest> = {}): RunRequest => ({
  job_id: 'job-1',
  type: 'qa_run',
  params: { roles: ['admin'], max_steps: 1, interactive_skills: false, flows: [] },
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

test('parseCredentials keeps only complete entries', () => {
  const c = parseCredentials(JSON.stringify({ a: { username: 'u', password: 'p' }, b: { username: 'u' }, c: null }));
  assert.deepEqual(Object.keys(c), ['a']);
  assert.throws(() => parseCredentials('[]'));
});

test('parseRunRequest validates type, roles and max_steps', () => {
  const ok = parseRunRequest({ type: 'qa_run', job_id: 'j', params: { roles: ['cashier'], max_steps: 5 } }, ROLES, ['admin']);
  assert.ok(ok.ok);
  assert.deepEqual(ok.ok && ok.value.params, { roles: ['cashier'], max_steps: 5, interactive_skills: false, flows: [] });

  const def = parseRunRequest({ type: 'docs_generate' }, ROLES, ['admin']);
  assert.ok(def.ok && (def.value.params as RunParams).roles[0] === 'admin' && (def.value.params as RunParams).max_steps === 10);

  for (const bad of [
    null,
    [],
    { type: 'rm -rf' },
    { type: 'qa_run', params: { roles: ['root'] } },
    { type: 'qa_run', params: { roles: 'admin' } },
    { type: 'qa_run', params: { max_steps: 0 } },
    { type: 'qa_run', params: { max_steps: 51 } },
    { type: 'qa_run', params: { max_steps: 2.5 } },
    { type: 'qa_run', job_id: 'j'.repeat(129) },
    { type: 'qa_run', params: { roles: ['a', 'b', 'c', 'd', 'e', 'f'] } },
  ]) {
    assert.equal(parseRunRequest(bad, [...ROLES, 'a', 'b', 'c', 'd', 'e', 'f'], ['admin']).ok, false, JSON.stringify(bad));
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
  const cfg: ExecutorConfig = {
    bind: '127.0.0.1',
    port: 0,
    token: TOKEN,
    stagingUrl: 'https://staging.invalid',
    credentials: { admin: { username: 'u', password: 'p' } },
    defaultRoles: ['admin'],
    outputDir: '/tmp/unused',
    retentionDays: 7,
    runTimeoutMs: 1000,
    maxQueue: 1,
    headless: true,
    repoPool: '/tmp/unused',
    envDir: '/tmp/unused',
    allowedHosts: ['staging.invalid'],
  };
  const q = new RunQueue(async () => ({ done: true }), { maxQueue: 1, timeoutMs: 1000, ttlMs: 60_000 });
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

    const created = await fetch(`${base}/v1/runs`, { method: 'POST', headers: auth, body: JSON.stringify({ type: 'qa_run', job_id: 'j1' }) });
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
  } finally {
    server.close();
  }
});
