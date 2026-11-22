/**
 * Navigation map API (docs/designs/NAVIGATION_MAP.md). Read-only: logs in as the impersonator
 * (E2E_IMPERSONATOR_USERNAME / _PASSWORD) and reads the SUT map. The map exists after the first
 * knowledge sync that ran with this version; the route checks skip until then.
 */

import { test, expect, APIRequestContext } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword } = config;
const json = { 'Content-Type': 'application/json' };

test.describe('Navigation map API', () => {
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
    base = `/api/products/${sut.id}/navigation`;
  });

  test.afterEach(async () => {
    await api.post('/api/auth/logout', { headers: json, data: {} });
    await api.dispose();
  });

  test('bad filters are refused', async () => {
    expect((await api.get(`${base}?surface=graphql`)).status()).toBe(400);
    expect((await api.get(`${base}?status=green`)).status()).toBe(400);
    expect((await api.get(`${base}/routes/nope`)).status()).toBe(404);
  });

  test('sut-vendor-backend routes and sut-frontend pages are mapped with coverage', async () => {
    const res = await api.get(`${base}?surface=api&app=sut-vendor-backend`);
    expect(res.status()).toBe(200);
    const map = await res.json();
    test.skip(!map.build, 'no navigation build yet (run a knowledge sync)');
    const paths: string[] = map.routes.map((r: { path: string }) => r.path);
    expect(paths).toContain('/api/v1/service-status');
    expect(paths).toContain('/api/v1/swagger/*');
    expect(paths.some((p) => p.startsWith('/api/v1/kiosk-sut-v2/'))).toBe(true);
    const probe = map.routes.find((r: { path: string }) => r.path === '/api/v1/service-status');
    expect(probe.cases).toBeGreaterThan(0);
    expect(['failing', 'passing', 'automated', 'planned', 'untested']).toContain(probe.status);

    const detail = await (await api.get(`${base}/routes/${probe.key}`)).json();
    expect(detail.cases.map((c: { code: string }) => c.code)).toContain('TC-VSYS-001');

    const web = await (await api.get(`${base}?surface=web`)).json();
    const pages: string[] = web.routes.map((r: { path: string }) => r.path);
    expect(pages).toContain('/auth/login');
    expect(pages).toContain('/report-builder/list');
  });
});
