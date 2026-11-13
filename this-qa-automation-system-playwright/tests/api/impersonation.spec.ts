/**
 * Impersonation API tests (docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md).
 *
 * Needs an impersonator account (role Admin or Service) and a target user:
 *   E2E_IMPERSONATOR_USERNAME, E2E_IMPERSONATOR_PASSWORD, E2E_TARGET_USERNAME
 * The target should be a QA/QC user so the "QA cannot manage products" check
 * holds. Skipped when the variables are unset.
 *
 * Creates no data, except one impersonation_audit row per run. Audit rows are
 * kept by design (accepted exception to the clean-up rule).
 */

import { test, expect, APIRequestContext } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword, impersonationTarget } = config;
const json = { 'Content-Type': 'application/json' };

test.describe('Impersonation API', () => {
  test.skip(!impersonator || !impersonatorPassword || !impersonationTarget,
    'E2E_IMPERSONATOR_USERNAME / E2E_IMPERSONATOR_PASSWORD / E2E_TARGET_USERNAME not set');

  let api: APIRequestContext;

  test.beforeEach(async ({ playwright }) => {
    // Own context so the session cookie is isolated per test.
    api = await playwright.request.newContext({ baseURL: config.backendUrl });
    const login = await api.post('/api/auth/login', {
      headers: json,
      data: { username: impersonator, password: impersonatorPassword },
    });
    expect(login.status()).toBe(200);
  });

  test.afterEach(async () => {
    // Always leave clean: stop (400 if not impersonating is fine), then log out.
    await api.post('/api/auth/impersonate/stop', { headers: json, data: {} });
    await api.post('/api/auth/logout', { headers: json, data: {} });
    await api.dispose();
  });

  test('GET /api/users lists users without credentials', async () => {
    const res = await api.get('/api/users');
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body)).toBe(true);
    expect(JSON.stringify(body).toLowerCase()).not.toContain('password');
    expect(body.some((u: { username: string }) => u.username === impersonationTarget)).toBe(true);
  });

  test('impersonate → act as target → stop', async () => {
    const start = await api.post('/api/auth/impersonate', { headers: json, data: { username: impersonationTarget } });
    expect(start.status()).toBe(200);
    const started = await start.json();
    expect(started.user.username).toBe(impersonationTarget);
    expect(started.user.impersonator.username).toBe(impersonator);
    expect(started.mustChangePassword).toBe(false);

    const me = await (await api.get('/api/auth/me')).json();
    expect(me.user.impersonator.username).toBe(impersonator);

    // No nesting, no password change while impersonating.
    expect((await api.post('/api/auth/impersonate', { headers: json, data: { username: impersonator } })).status()).toBe(403);
    expect((await api.post('/api/auth/change-password', {
      headers: json, data: { currentPassword: 'x', newPassword: 'yyyyyyyyyy' },
    })).status()).toBe(403);

    // A QA/QC target cannot manage products (rejected before any write).
    expect((await api.post('/api/products', { headers: json, data: { name: 'e2e-should-not-exist' } })).status()).toBe(403);

    const stop = await api.post('/api/auth/impersonate/stop', { headers: json, data: {} });
    expect(stop.status()).toBe(200);
    const stopped = await stop.json();
    expect(stopped.user.username).toBe(impersonator);
    expect(stopped.user.impersonator).toBeUndefined();
  });

  test('rejects non-JSON content type and self-impersonation', async () => {
    const form = await api.post('/api/auth/impersonate', {
      headers: { 'Content-Type': 'text/plain' },
      data: JSON.stringify({ username: impersonationTarget }),
    });
    expect(form.status()).toBe(415);

    const self = await api.post('/api/auth/impersonate', { headers: json, data: { username: impersonator } });
    expect(self.status()).toBe(400);
  });
});
