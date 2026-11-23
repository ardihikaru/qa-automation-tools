/**
 * The Brain Town reporter (reporters/brain-town-reporter.cjs): resolver and config, how case_run
 * wires it, and the reporter in a real Playwright run against a local receiver.
 */
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import * as fs from 'node:fs';
import * as http from 'node:http';
import * as os from 'node:os';
import * as path from 'node:path';
import { test } from 'node:test';
import { BRAIN_TOWN_REPORTER_PATH, btVar, loadBrainTownConfig, parseProductMap, productFor, resolveBrainTownReporter } from '../src/brainTownReporter';

test('resolveBrainTownReporter: needs BRAIN_TOWN_URL, honours the off flag, bad files are off', () => {
  assert.ok(fs.existsSync(BRAIN_TOWN_REPORTER_PATH));
  assert.equal(resolveBrainTownReporter({}), '');
  assert.equal(resolveBrainTownReporter({ BRAIN_TOWN_URL: 'http://x/y' }), BRAIN_TOWN_REPORTER_PATH);
  assert.equal(resolveBrainTownReporter({ BRAIN_TOWN_URL: 'http://x/y', EXECUTOR_BRAIN_TOWN_REPORTER: '0' }), '');
  assert.equal(resolveBrainTownReporter({ BRAIN_TOWN_URL: 'http://x/y' }, '/nonexistent/r.cjs'), '');
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'bt-rep-'));
  fs.writeFileSync(path.join(dir, 'bad.cjs'), 'module.exports = 42;');
  assert.equal(resolveBrainTownReporter({ BRAIN_TOWN_URL: 'http://x/y' }, path.join(dir, 'bad.cjs')), '');
  assert.equal(resolveBrainTownReporter({ BRAIN_TOWN_URL: 'http://x/y' }, path.join(dir, 'a,b.cjs')), '');
});

test('product mapping: map entry, default, then product-<id>', () => {
  assert.deepEqual(parseProductMap('1=samplereport, 2 = sut,bad,3=has space'), { 1: 'samplereport', 2: 'sut' });
  const cfg = loadBrainTownConfig({ BRAIN_TOWN_URL: 'http://x', BRAIN_TOWN_PRODUCTS: '1=samplereport', BRAIN_TOWN_PRODUCT: 'dflt' });
  assert.equal(cfg.org, 'example-org');
  assert.equal(productFor(cfg, 'automation-1.env'), 'samplereport');
  assert.equal(productFor(cfg, 'automation-7.env'), 'dflt');
  assert.equal(productFor({ ...cfg, product: '' }, 'automation-7.env'), 'product-7');
});

test('per-instance vars: NAME upper-cased, - becomes _, named instance never uses the default', () => {
  assert.equal(btVar('BRAIN_TOWN_URL', ''), 'BRAIN_TOWN_URL');
  assert.equal(btVar('BRAIN_TOWN_URL', 'personal'), 'BRAIN_TOWN_URL__PERSONAL');
  assert.equal(btVar('BRAIN_TOWN_TOKEN', 'my-site'), 'BRAIN_TOWN_TOKEN__MY_SITE');
  const env = {
    BRAIN_TOWN_URL: 'http://x/default', BRAIN_TOWN_TOKEN: 'bt_live_d', BRAIN_TOWN_ORG: 'dflt-org', BRAIN_TOWN_PRODUCT: 'dp',
    BRAIN_TOWN_URL__MY_SITE: 'http://x/site', BRAIN_TOWN_TOKEN__MY_SITE: ' bt_live_s ', BRAIN_TOWN_PRODUCT__MY_SITE: 'sp', BRAIN_TOWN_PRODUCTS__MY_SITE: '2=other',
  };
  const site = loadBrainTownConfig(env, BRAIN_TOWN_REPORTER_PATH, 'my-site');
  assert.deepEqual([site.url, site.token, site.org, site.product, site.products, site.reporter], ['http://x/site', 'bt_live_s', 'example-org', 'sp', { 2: 'other' }, BRAIN_TOWN_REPORTER_PATH]);
  assert.equal(productFor(site, 'automation-2.env'), 'other');
  // Default unchanged.
  const d = loadBrainTownConfig(env);
  assert.deepEqual([d.url, d.token, d.org, d.product], ['http://x/default', 'bt_live_d', 'dflt-org', 'dp']);
  // A named instance without its own URL is off, even with a default URL, token and kill switch unset.
  const none = loadBrainTownConfig(env, BRAIN_TOWN_REPORTER_PATH, 'personal');
  assert.deepEqual([none.reporter, none.url, none.token, none.product], ['', '', '', '']);
  assert.equal(resolveBrainTownReporter(env, BRAIN_TOWN_REPORTER_PATH, 'personal'), '');
  assert.equal(resolveBrainTownReporter({ ...env, EXECUTOR_BRAIN_TOWN_REPORTER: '0' }, BRAIN_TOWN_REPORTER_PATH, 'my-site'), '');
});

interface Ev { id: string; type: string; run: string; product?: string; org?: string; data: Record<string, unknown> }

async function playwrightRun(env: Record<string, string>, spec: string): Promise<{ ms: number; output: string; status: number | null }> {
  const cli = path.resolve(__dirname, '..', '..', 'node_modules', 'playwright', 'cli.js');
  const dir = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'bt-pw-')));
  fs.mkdirSync(path.join(dir, 'tests'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'playwright.config.js'), `module.exports = { testDir: './tests' };\n`);
  fs.writeFileSync(path.join(dir, 'tests/a.spec.js'),
    `const { test } = require(${JSON.stringify(path.resolve(__dirname, '..', '..', 'node_modules', 'playwright', 'test'))});\n${spec}`);
  const t0 = Date.now();
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [cli, 'test', `--reporter=list,${BRAIN_TOWN_REPORTER_PATH}`], { cwd: dir, env: { ...process.env, CI: '1', ...env } });
    let output = '';
    child.stdout.on('data', (d) => (output += d));
    child.stderr.on('data', (d) => (output += d));
    child.on('close', (status) => resolve({ ms: Date.now() - t0, output, status }));
  });
}

function receiver(): Promise<{ url: string; events: Ev[]; auth: string[]; close: () => void }> {
  const events: Ev[] = [];
  const auth: string[] = [];
  const server = http.createServer((req, res) => {
    let body = '';
    req.on('data', (d) => (body += d));
    req.on('end', () => {
      auth.push(String(req.headers.authorization));
      events.push(JSON.parse(body));
      res.end('{}');
    });
  });
  return new Promise((r) => server.listen(0, '127.0.0.1', () => r({ url: `http://127.0.0.1:${(server.address() as { port: number }).port}/ingest`, events, auth, close: () => server.close() })));
}

const PASS = `test('x', async () => {
  await test.step('Open page', async () => {});
  await test.step('Sign in', async () => { await test.step('inner', async () => {}); });
  await test.step('Check result', async () => {});
});\n`;
const FAIL = `test('x', async () => {
  await test.step('Open page', async () => {});
  await test.step('Sign in', async () => { throw new Error('secret-stack-text'); });
});\n`;

test('real run: started, one step per test.step, finished; bearer token; no error text', async (t) => {
  if (!fs.existsSync(path.resolve(__dirname, '..', '..', 'node_modules', 'playwright', 'cli.js'))) return t.skip('playwright is not installed');
  const rx = await receiver();
  try {
    const base = { BRAIN_TOWN_URL: rx.url, BRAIN_TOWN_TOKEN: 'bt_live_test', BRAIN_TOWN_PRODUCT: 'samplereport', BRAIN_TOWN_ORG: 'example-org', VENDOR_E2E_RUN: 'qaabc123' };
    const ok = await playwrightRun(base, PASS);
    assert.equal(ok.status, 0, ok.output);
    assert.deepEqual(rx.events.map((e) => e.type), ['qa.run.started', 'qa.step', 'qa.step', 'qa.step', 'qa.run.finished']);
    assert.deepEqual(rx.events[0].data.steps, ['Open page', 'Sign in', 'inner', 'Check result']);
    assert.deepEqual(rx.events.slice(1, 4).map((e) => [e.data.index, e.data.step, e.data.status]), [[1, 'Open page', 'passed'], [2, 'Sign in', 'passed'], [3, 'Check result', 'passed']]);
    assert.ok(rx.events.every((e) => e.run === 'qaabc123' && e.product === 'samplereport' && e.org === 'example-org'));
    assert.deepEqual(rx.events[4].data, { ...rx.events[4].data, passed: 1, failed: 0, total: 1 });
    assert.ok(rx.auth.every((a) => a === 'Bearer bt_live_test'));

    rx.events.length = 0;
    const bad = await playwrightRun(base, FAIL);
    assert.equal(bad.status, 1);
    const steps = rx.events.filter((e) => e.type === 'qa.step');
    assert.deepEqual(steps.map((e) => e.data.status), ['passed', 'failed']);
    const fin = rx.events[rx.events.length - 1];
    assert.equal(fin.type, 'qa.run.finished');
    assert.equal(fin.data.failed, 1);
    assert.ok(!JSON.stringify(rx.events).includes('secret-stack-text'));
  } finally {
    rx.close();
  }
});

test('real run: URL unset makes no request; unreachable Brain Town adds at most ~2 s', async (t) => {
  if (!fs.existsSync(path.resolve(__dirname, '..', '..', 'node_modules', 'playwright', 'cli.js'))) return t.skip('playwright is not installed');
  const rx = await receiver();
  try {
    const off = await playwrightRun({ BRAIN_TOWN_URL: '' }, PASS);
    assert.equal(off.status, 0, off.output);
    assert.equal(rx.events.length, 0);
    const baseline = off.ms;
    // A black hole: connect never completes, so only the 2 s timeout ends each attempt.
    const dead = await playwrightRun({ BRAIN_TOWN_URL: 'http://10.255.255.1:81/ingest', BRAIN_TOWN_TOKEN: 'x' }, PASS);
    assert.equal(dead.status, 0, dead.output);
    assert.ok(dead.ms - baseline < 3500, `took ${dead.ms - baseline} ms longer`);
  } finally {
    rx.close();
  }
});
