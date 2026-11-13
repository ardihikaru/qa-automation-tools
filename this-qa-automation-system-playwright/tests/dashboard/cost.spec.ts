/**
 * Cost Analytics view (/cost): AI call ledger + ai-manager gateway.
 * Needs a login: set QA_USERNAME and QA_PASSWORD (read-only; nothing is created).
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

const ENDPOINTS = ['overview', 'trend', 'efficiency', 'by-feature', 'by-model', 'recent', 'filters', 'gateway'];

test.describe('Cost Analytics View (/cost)', () => {
  test.skip(!process.env.QA_USERNAME || !process.env.QA_PASSWORD, 'set QA_USERNAME and QA_PASSWORD');

  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/login`);
    const inputs = page.locator('input');
    await inputs.nth(0).fill(process.env.QA_USERNAME!);
    await inputs.nth(1).fill(process.env.QA_PASSWORD!);
    await page.click('button[type=submit]');
    await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  });

  test('loads every cost endpoint and shows no error banner', async ({ page }) => {
    const seen = new Map<string, number>();
    page.on('response', (r) => {
      const m = new URL(r.url()).pathname.match(/^\/api\/cost\/([\w-]+)$/);
      if (m) seen.set(m[1], r.status());
    });
    await page.goto(`${config.dashboardUrl}/cost`);
    await expect(page.getByRole('heading', { name: 'Cost Analytics' })).toBeVisible();
    await expect.poll(() => ENDPOINTS.every((e) => seen.has(e)), { timeout: 15000 }).toBe(true);
    for (const e of ENDPOINTS) expect(seen.get(e), e).toBe(200);
    await expect(page.getByRole('alert')).toHaveCount(0);
  });

  test('date presets and the failed-only toggle re-query with filters', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/cost`);
    await expect(page.getByRole('heading', { name: 'Cost Analytics' })).toBeVisible();

    const preset = page.getByRole('group', { name: 'Date range presets' }).getByRole('button', { name: /7/ });
    const [overview] = await Promise.all([
      page.waitForResponse((r) => r.url().includes('/api/cost/overview') && r.url().includes('from=')),
      preset.click(),
    ]);
    expect(overview.status()).toBe(200);

    const [recent] = await Promise.all([
      page.waitForResponse((r) => r.url().includes('/api/cost/recent') && r.url().includes('status=error')),
      page.getByLabel('Failed only').check(),
    ]);
    expect(recent.status()).toBe(200);
  });

  test('has no console errors', async ({ page }) => {
    // The post-login landing page (/docs) may still be fetching; let it settle so its aborted
    // request is not blamed on /cost.
    await page.waitForLoadState('networkidle');
    const errors: string[] = [];
    page.on('console', (m) => {
      if (m.type() === 'error' && !/favicon|404|net::ERR/.test(m.text())) errors.push(m.text());
    });
    await page.goto(`${config.dashboardUrl}/cost`);
    await expect(page.getByRole('heading', { name: 'Cost Analytics' })).toBeVisible();
    await page.waitForLoadState('networkidle');
    expect(errors).toEqual([]);
  });
});
