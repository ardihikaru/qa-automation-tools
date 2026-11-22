/**
 * Product knowledge base API tests (docs/designs/PRODUCT_KNOWLEDGE_SYNC.md).
 *
 * Read-only by default: logs in as the impersonator (E2E_IMPERSONATOR_USERNAME / _PASSWORD)
 * and reads the SUT knowledge base. Set E2E_KB_SYNC_AS=<username of an Admin/DevOps/
 * Developer/QA/QC user> to also impersonate that user and run one sync (a write that adds one
 * kb_syncs row; kept by design, like audit rows).
 */

import { test, expect, APIRequestContext } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword } = config;
const syncAs = process.env.E2E_KB_SYNC_AS || '';
const json = { 'Content-Type': 'application/json' };

test.describe('Knowledge base API', () => {
  test.skip(!impersonator || !impersonatorPassword, 'E2E_IMPERSONATOR_USERNAME / E2E_IMPERSONATOR_PASSWORD not set');

  let api: APIRequestContext;
  let base = '';

  test.beforeEach(async ({ playwright }) => {
    api = await playwright.request.newContext({ baseURL: config.backendUrl });
    const login = await api.post('/api/auth/login', { headers: json, data: { username: impersonator, password: impersonatorPassword } });
    expect(login.status()).toBe(200);
    const { products } = await (await api.get('/api/products')).json();
    const sut = products.find((p: { name: string }) => p.name === (process.env.QA_PRODUCT_NAME || 'SUT'));
    test.skip(!sut, 'no SUT product');
    base = `/api/products/${sut.id}/knowledge`;
  });

  test.afterEach(async () => {
    await api.post('/api/auth/impersonate/stop', { headers: json, data: {} });
    await api.post('/api/auth/logout', { headers: json, data: {} });
    await api.dispose();
  });

  test('summary, tree and lists are readable', async () => {
    const sum = await (await api.get(`${base}/summary`)).json();
    expect(sum.configured).toBe(true);
    expect(sum.enums.status).toContain('automated');
    test.skip(!sum.last_success, 'no successful sync yet');
    expect(sum.counts.test_cases).toBeGreaterThan(0);

    const tree = await (await api.get(`${base}/tree`)).json();
    expect(tree.parts.length).toBeGreaterThan(0);

    const tcs = await (await api.get(`${base}/test-cases?status=automated&page_size=5`)).json();
    expect(tcs.items.length).toBeLessThanOrEqual(5);
    for (const tc of tcs.items) expect(tc.automated).toBe(true);
  });

  // EPIC_AUTOMATION.md: read-only; starting a batch spends LLM budget and is never done here.
  test('an epic reports what could be automated, and bad epics are refused', async () => {
    const epics = await (await api.get(`${base}/epics?page_size=1`)).json();
    test.skip(!epics.epics?.length, 'no epics synced');
    const code = epics.epics[0].code as string;
    const res = await api.get(`${base}/epics/${encodeURIComponent(code)}/automation`);
    expect(res.status()).toBe(200);
    const out = await res.json();
    expect(typeof out.repo).toBe('boolean');
    expect(out.eligible).toBeGreaterThanOrEqual(0);
    expect(Array.isArray(out.sample)).toBe(true);
    const bad = await api.get(`${base}/epics/NO-SUCH-EPIC-X/automation`);
    expect(bad.status()).toBe(400);
    expect((await bad.json()).code).toBe('unknown_epic');
  });

  test('rejects bad filters and unknown codes', async () => {
    expect((await api.get(`${base}/test-cases?level=BOGUS`)).status()).toBe(400);
    expect((await api.get(`${base}/stories/US-NOPE-99`)).status()).toBe(404);
    expect((await api.get(`${base}/nope`)).status()).toBe(404);
  });

  test('sync as a write role', async () => {
    test.skip(!syncAs, 'E2E_KB_SYNC_AS not set');
    const imp = await api.post('/api/auth/impersonate', { headers: json, data: { username: syncAs } });
    expect(imp.status()).toBe(200);
    const start = await api.post(`${base}/sync`, { headers: json, data: {} });
    expect([202, 409]).toContain(start.status());
    const id = (await start.json()).sync_id;
    let sync: { status: string; ai_status: string } = { status: 'running', ai_status: 'none' };
    for (let i = 0; i < 120 && (sync.status === 'running'); i++) {
      await new Promise((r) => setTimeout(r, 1000));
      sync = await (await api.get(`${base}/syncs/${id}`)).json();
    }
    expect(sync.status).toBe('succeeded');
  });
});
