/**
 * Run history API (docs/designs/RUN_HISTORY.md). Read-only: logs in as the impersonator
 * (E2E_IMPERSONATOR_USERNAME / _PASSWORD) and reads the three lists. Writes nothing.
 */

import { test, expect, APIRequestContext } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword } = config;
const json = { 'Content-Type': 'application/json' };

test.describe('Run history API', () => {
  test('needs a session', async ({ request }) => {
    const res = await request.get(`${config.backendUrl}/api/history/builds`);
    expect(res.status()).toBe(401);
  });

  test.describe('signed in', () => {
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

    test('builds carry queue, machine and waiting time and their steps', async () => {
      const res = await api.get('/api/history/builds?page_size=5');
      expect(res.status()).toBe(200);
      const body = await res.json();
      for (const k of ['items', 'total', 'page', 'page_size', 'summary']) expect(body).toHaveProperty(k);
      expect(body.page_size).toBe(5);
      expect(body.items.length).toBeLessThanOrEqual(5);
      for (const b of body.items) {
        for (const k of ['id', 'type', 'status', 'queue_ms', 'active_ms', 'waiting_ms', 'machine_ms', 'steps', 'case_code', 'product_id']) expect(b).toHaveProperty(k);
        expect(['case_create', 'case_run']).toContain(b.type);
        expect(b.waiting_ms).toBeGreaterThanOrEqual(0);
        if (b.active_ms !== null) expect(b.machine_ms + b.waiting_ms).toBeLessThanOrEqual(b.active_ms + 1);
      }
    });

    test('Playwright runs and a run detail', async () => {
      const res = await api.get('/api/history/runs?page_size=5');
      expect(res.status()).toBe(200);
      const body = await res.json();
      for (const k of ['items', 'total', 'page', 'page_size', 'summary', 'since']) expect(body).toHaveProperty(k);
      test.skip(body.items.length === 0, 'no Playwright run recorded yet');
      const run = body.items[0];
      for (const k of ['source', 'run_ref', 'started_at', 'duration_ms', 'passed', 'failed', 'flaky', 'skipped', 'cases', 'outcome']) expect(run).toHaveProperty(k);
      const det = await (await api.get(`/api/history/runs/${run.id}`)).json();
      expect(Array.isArray(det.case_rows)).toBe(true);
      expect(det.case_rows.length).toBe(run.cases);
    });

    test('knowledge syncs and bad parameters', async () => {
      const syncs = await api.get('/api/history/syncs?page_size=5');
      expect(syncs.status()).toBe(200);
      expect(await syncs.json()).toHaveProperty('items');
      expect((await api.get('/api/history/builds?product=x')).status()).toBe(400);
      expect((await api.get('/api/history/runs?source=nope')).status()).toBe(400);
      expect((await api.get('/api/history/builds?from=2026-11-20&to=2026-11-01')).status()).toBe(400);
      expect((await api.get('/api/history/runs/999999999')).status()).toBe(404);
    });

    // HISTORY_ANSWER.md: approvals waiting now, read-only here.
    test('waiting approvals carry the context to decide', async () => {
      const res = await api.get('/api/history/approvals');
      expect(res.status()).toBe(200);
      const body = await res.json();
      expect(Array.isArray(body.items)).toBe(true);
      expect(typeof body.can_answer).toBe('boolean');
      for (const it of body.items) {
        for (const k of ['external_id', 'round', 'job_id', 'job_type', 'product_id', 'case_code', 'title', 'spec_path', 'report', 'gaos_created', 'relay', 'created_at', 'waiting_ms'])
          expect(it).toHaveProperty(k);
        expect(it.external_id).toMatch(/^qa-.+-pr[0-9]*$/);
      }
      expect((await api.get('/api/history/approvals?product=x')).status()).toBe(400);
      expect((await api.get('/api/history/builds?approval=closed')).status()).toBe(400);
      const open = await (await api.get('/api/history/builds?approval=open&page_size=5')).json();
      for (const b of open.items) expect(b.approval?.status).toBe('open');
    });
  });
});
