/**
 * AI conversations API (docs/designs/AI_CONVERSATIONS.md). Read-only: logs in as the impersonator
 * (E2E_IMPERSONATOR_USERNAME / _PASSWORD), reads the lists and one conversation, and checks the
 * refusals. It never archives or prunes.
 */

import { test, expect, APIRequestContext } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword } = config;
const json = { 'Content-Type': 'application/json' };

test.describe('AI conversations API', () => {
  test('needs a session', async ({ request }) => {
    expect((await request.get(`${config.backendUrl}/api/conversations`)).status()).toBe(401);
  });

  test.describe('signed in', () => {
    test.skip(!impersonator || !impersonatorPassword, 'E2E_IMPERSONATOR_USERNAME / E2E_IMPERSONATOR_PASSWORD not set');

    let api: APIRequestContext;
    let role = '';
    let impersonating = false;

    test.beforeEach(async ({ playwright }) => {
      api = await playwright.request.newContext({ baseURL: config.backendUrl });
      const login = await api.post('/api/auth/login', { headers: json, data: { username: impersonator, password: impersonatorPassword } });
      expect(login.status()).toBe(200);
      const me = (await login.json()).user;
      role = me.role;
      impersonating = !!me.impersonator;
    });

    test.afterEach(async () => {
      await api.post('/api/auth/logout', { headers: json, data: {} });
      await api.dispose();
    });

    test('lists and one conversation', async () => {
      const res = await api.get('/api/conversations?page_size=5');
      expect(res.status()).toBe(200);
      const body = await res.json();
      for (const k of ['items', 'total', 'page', 'page_size', 'totals']) expect(body).toHaveProperty(k);
      expect((await api.get('/api/conversations?archived=1&page_size=1')).status()).toBe(200);
      test.skip(body.items.length === 0, 'no conversation stored yet');
      const c = body.items[0];
      for (const k of ['id', 'kind', 'ref', 'title', 'message_count', 'bytes', 'archived_at']) expect(c).toHaveProperty(k);
      expect(c.archived_at).toBeNull();
      const det = await api.get(`/api/conversations/${c.id}`);
      if (['Admin', 'DevOps', 'Developer', 'QA', 'QC'].includes(role)) {
        expect(det.status()).toBe(200);
        const d = await det.json();
        expect(d.messages.length).toBe(c.message_count);
        expect(d.messages[0].seq).toBe(1);
      } else {
        expect(det.status()).toBe(403);
      }
    });

    test('refusals', async () => {
      expect((await api.get('/api/conversations?kind=nope')).status()).toBe(400);
      expect((await api.get('/api/conversations/999999999')).status()).toBeGreaterThanOrEqual(403);
      const job = await api.get('/api/jobs/job-does-not-exist/conversation');
      expect(job.status()).toBe(200);
      expect((await job.json()).conversation).toBeNull();
      const prune = await api.post('/api/conversations/prune', { headers: json, data: { confirm: 'nope' } });
      if (role === 'Admin' && !impersonating) expect(prune.status()).toBe(400);
      else expect(prune.status()).toBe(403);
    });
  });
});
