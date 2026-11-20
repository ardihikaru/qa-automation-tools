/**
 * Learning loop API (docs/designs/LEARNING_LOOP.md).
 *
 * Read-only: logs in as the impersonator (E2E_IMPERSONATOR_USERNAME / _PASSWORD) and reads
 * /api/learning/{findings,proposals,timeline}. Shapes are asserted for any data; the product
 * filter is checked against the HISv3 product when it exists. Nothing is created, so no
 * proposal, approval or pull request is ever started from here.
 */

import { test, expect, APIRequestContext } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword } = config;
const json = { 'Content-Type': 'application/json' };

test.describe('Learning loop API', () => {
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

  test('the learning routes require a session', async ({ playwright }) => {
    const anon = await playwright.request.newContext({ baseURL: config.backendUrl });
    for (const p of ['findings', 'proposals', 'timeline']) {
      expect((await anon.get(`/api/learning/${p}`)).status(), p).toBe(401);
    }
    await anon.dispose();
  });

  test('findings, proposals and the timeline have their documented shape', async () => {
    const f = await api.get('/api/learning/findings?limit=5');
    expect(f.status()).toBe(200);
    const fb = await f.json();
    expect(Array.isArray(fb.items)).toBe(true);
    expect(typeof fb.total).toBe('number');
    for (const k of ['new', 'proposed', 'learned', 'covered', 'dismissed']) expect(typeof fb.counts[k], k).toBe('number');

    const p = await api.get('/api/learning/proposals?limit=5');
    expect(p.status()).toBe(200);
    const pb = await p.json();
    expect(Array.isArray(pb.items)).toBe(true);
    for (const k of ['drafting', 'awaiting_approval', 'pr_open', 'learned', 'failed']) expect(typeof pb.counts[k], k).toBe('number');

    const t = await api.get('/api/learning/timeline?limit=5');
    expect(t.status()).toBe(200);
    expect(Array.isArray((await t.json()).items)).toBe(true);
  });

  test('the product filter scopes findings', async () => {
    const { products } = await (await api.get('/api/products')).json();
    const his = (products ?? []).find((x: { name: string }) => x.name === 'HISv3');
    test.skip(!his, 'no HISv3 product');
    const res = await api.get(`/api/learning/findings?product_id=${his.id}&limit=50`);
    expect(res.status()).toBe(200);
    for (const it of (await res.json()).items) expect(it.product_id).toBe(his.id);
  });

  test('bad filters are refused, legacy routes still answer', async () => {
    expect((await api.get('/api/learning/findings?status=bogus')).status()).toBe(400);
    expect((await api.get('/api/learning/proposals?state=bogus')).status()).toBe(400);
    expect((await api.get('/api/learning/findings/0')).status()).toBe(404);
    // /navigation still reads the crawler-era route; its shape is unchanged.
    const legacy = await api.get('/api/learning?limit=1');
    expect(legacy.status()).toBe(200);
    expect(Array.isArray(await legacy.json())).toBe(true);
  });
});
