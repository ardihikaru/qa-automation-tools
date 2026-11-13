/**
 * Cost Analytics API tests (AI call ledger + ai-manager gateway view).
 *
 * Read-only: logs in as the impersonator (E2E_IMPERSONATOR_USERNAME / _PASSWORD) and reads
 * /api/cost/*. The ledger may be empty on a fresh database, so shapes are asserted
 * unconditionally and values only through invariants that hold for any data.
 */

import { test, expect, APIRequestContext } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword } = config;
const json = { 'Content-Type': 'application/json' };

test.describe('Cost Analytics API', () => {
  test.skip(!impersonator || !impersonatorPassword, 'E2E_IMPERSONATOR_USERNAME / E2E_IMPERSONATOR_PASSWORD not set');

  let api: APIRequestContext;

  test.beforeEach(async ({ playwright }) => {
    api = await playwright.request.newContext({ baseURL: config.backendUrl });
    const login = await api.post('/api/auth/login', { headers: json, data: { username: impersonator, password: impersonatorPassword } });
    expect(login.status()).toBe(200);
  });

  test.afterEach(async () => {
    await api.post('/api/auth/logout', { headers: json, data: {} });
    await api.dispose();
  });

  test('cost endpoints require a session', async ({ playwright }) => {
    const anon = await playwright.request.newContext({ baseURL: config.backendUrl });
    for (const p of ['overview', 'trend', 'by-feature', 'by-model', 'recent', 'gateway']) {
      expect((await anon.get(`/api/cost/${p}`)).status(), p).toBe(401);
    }
    await anon.dispose();
  });

  test('overview has consistent totals', async () => {
    const res = await api.get('/api/cost/overview');
    expect(res.status()).toBe(200);
    expect(res.headers()['content-type']).toContain('application/json');
    const o = await res.json();
    for (const k of ['total_requests', 'success_calls', 'error_calls', 'error_rate', 'fallback_calls', 'total_tokens',
      'input_tokens', 'output_tokens', 'total_cost', 'total_cost_idr', 'estimated_cost', 'avg_latency_ms', 'p95_latency_ms']) {
      expect(typeof o[k], k).toBe('number');
    }
    expect(o.success_calls + o.error_calls).toBe(o.total_requests);
    expect(o.total_tokens).toBe(o.input_tokens + o.output_tokens);
    expect(o.error_rate).toBeGreaterThanOrEqual(0);
    expect(o.error_rate).toBeLessThanOrEqual(1);
  });

  test('trend is zero-filled for the requested range', async () => {
    const res = await api.get('/api/cost/trend?from=2026-01-01&to=2026-01-10');
    expect(res.status()).toBe(200);
    const days = await res.json();
    expect(days.map((d: { date: string }) => d.date)).toEqual(
      Array.from({ length: 10 }, (_, i) => `2026-01-${String(i + 1).padStart(2, '0')}`));
    for (const d of days) {
      expect(d.calls).toBeGreaterThanOrEqual(d.errors);
      expect(typeof d.cost).toBe('number');
    }
  });

  test('breakdowns, efficiency and filters have the documented shape', async () => {
    for (const p of ['by-feature', 'by-model']) {
      const rows = await (await api.get(`/api/cost/${p}`)).json();
      expect(Array.isArray(rows), p).toBe(true);
      for (const r of rows) {
        expect(r.calls).toBeGreaterThan(0);
        expect(r.errors).toBeLessThanOrEqual(r.calls);
        expect(typeof r.cost).toBe('number');
      }
    }
    const eff = await (await api.get('/api/cost/efficiency')).json();
    expect(eff.success + eff.errors).toBe(eff.calls);
    const f = await (await api.get('/api/cost/filters')).json();
    for (const k of ['features', 'providers', 'models']) expect(Array.isArray(f[k]), k).toBe(true);
  });

  test('recent calls are paginated, filterable and show errors', async () => {
    const page = await (await api.get('/api/cost/recent?limit=5')).json();
    expect(page.items.length).toBeLessThanOrEqual(5);
    expect(page.total).toBeGreaterThanOrEqual(page.items.length);
    expect((await (await api.get('/api/cost/recent?limit=9999')).json()).limit).toBe(200);
    const failed = await (await api.get('/api/cost/recent?status=error&limit=20')).json();
    for (const c of failed.items) {
      expect(c.status).toBe('error');
      expect(c.error.length).toBeGreaterThan(0);
    }
  });

  test('rejects bad filters and methods', async () => {
    expect((await api.get('/api/cost/overview?from=nope')).status()).toBe(400);
    expect((await api.get('/api/cost/overview?status=maybe')).status()).toBe(400);
    expect((await api.get('/api/cost/overview?from=2026-02-01&to=2026-01-01')).status()).toBe(400);
    expect((await api.get('/api/cost/gateway?month=13-2026')).status()).toBe(400);
    expect((await api.post('/api/cost/overview', { headers: json, data: {} })).status()).toBe(405);
  });

  test('gateway answers 200 whether or not ai-manager is reachable', async () => {
    const res = await api.get('/api/cost/gateway');
    expect(res.status()).toBe(200);
    const g = await res.json();
    expect(typeof g.available).toBe('boolean');
    if (g.available) {
      expect(Array.isArray(g.usage.vendors)).toBe(true);
      expect(g.usage.total_calls).toBeGreaterThanOrEqual(g.usage.total_errors);
    } else {
      expect(g.error.length).toBeGreaterThan(0);
    }
  });
});
