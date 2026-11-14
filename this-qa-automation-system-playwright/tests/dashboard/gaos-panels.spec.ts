/**
 * Dashboard panels added for the GAOS integration (docs/designs/GAOS_INBOUND.md, CASE_RESULTS.md):
 * the "GAOS decisions" panel and the "Test results" upload card in the product detail.
 *
 * Read-only against production: logs in as the service account, impersonates E2E_TARGET_USERNAME
 * (a QA user), opens the SUT product, and checks both panels. The upload POST is answered by the
 * browser (page.route), so nothing is written. Env as tests/dashboard/impersonation.spec.ts, plus
 * QA_PRODUCT_NAME (default "SUT"). Skipped when unset.
 */

import { test, expect, Page } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword, impersonationTarget } = config;
const productName = process.env.QA_PRODUCT_NAME || 'SUT';
const shots = process.env.E2E_SCREENSHOT_DIR;

test.describe('GAOS panels in the product detail', () => {
  test.skip(!impersonator || !impersonatorPassword || !impersonationTarget, 'E2E_IMPERSONATOR_* / E2E_TARGET_USERNAME not set');

  async function openProduct(page: Page) {
    await page.goto(`${config.dashboardUrl}/login`);
    await page.fill('#login-username', impersonator);
    await page.fill('#login-password', impersonatorPassword);
    await page.click('button[type=submit]');
    await page.waitForURL((u) => !u.pathname.startsWith('/login'));
    await page.getByRole('button', { name: 'Log in as another user' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByPlaceholder('Filter by username or name').fill(impersonationTarget);
    await dialog.getByRole('button', { name: new RegExp(`^${impersonationTarget}\\b`, 'i') }).first().click();
    await expect(page.getByRole('status').filter({ hasText: 'Viewing as' })).toBeVisible({ timeout: 15000 });
    await page.goto(`${config.dashboardUrl}/products`);
    const row = page.getByRole('row').filter({ hasText: productName }).first();
    await row.getByRole('button', { name: /Details/ }).click();
  }

  async function cleanup(page: Page) {
    const h = { 'Content-Type': 'application/json' };
    await page.request.post(`${config.dashboardUrl}/api/auth/impersonate/stop`, { headers: h, data: {} });
    await page.request.post(`${config.dashboardUrl}/api/auth/logout`, { headers: h, data: {} });
  }

  for (const [name, viewport] of [['desktop', { width: 1440, height: 900 }], ['phone', { width: 375, height: 800 }]] as const) {
    test(`decisions panel and test results card render (${name})`, async ({ page }) => {
      await page.setViewportSize(viewport);
      try {
        await openProduct(page);

        // --- GAOS decisions ---
        const toggle = page.getByRole('button', { name: 'Show GAOS decisions' });
        await expect(toggle).toBeVisible({ timeout: 15000 });
        await expect(toggle).toHaveAttribute('aria-expanded', 'false');
        const resp = page.waitForResponse((r) => r.url().includes('/gaos/decisions') && r.request().method() === 'GET');
        await toggle.click();
        expect((await resp).status()).toBe(200);
        await expect(page.getByRole('button', { name: 'Hide GAOS decisions' })).toHaveAttribute('aria-expanded', 'true');
        await expect(page.getByText(/decisions from GAOS, \d+ differ/)).toBeVisible();
        await expect(page.getByLabel('Only cases that differ')).toBeVisible();
        if (shots) await page.screenshot({ path: `${shots}/decisions-${name}.png`, fullPage: true });
        // The filter can be flipped without an error.
        const only = page.getByLabel('Only cases that differ');
        await only.setChecked(!(await only.isChecked()));
        await expect(page.getByRole('alert')).toHaveCount(0);

        // --- Test results card ---
        const card = page.getByRole('form', { name: 'Test results' });
        await expect(card).toBeVisible();
        await expect(card.getByRole('button', { name: 'Upload' })).toBeDisabled();

        // Too large: refused in the browser, no request.
        let posted = 0;
        await page.route('**/case-results**', async (route) => {
          posted++;
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ tests: 12, cases: 3, run_ref: 'pw-1', run_at: '2026-11-14T10:00:00Z', stored: 2, stale: 1, unmatched: ['TC-ZZZ-999'], unmatched_count: 1 }),
          });
        });
        await card.locator('input[type=file]').setInputFiles({ name: 'big.json', mimeType: 'application/json', buffer: Buffer.alloc(25 * 1024 * 1024, 32) });
        await expect(card.getByRole('alert')).toBeVisible();
        expect(posted).toBe(0);

        // A normal file: the mocked answer is summarised.
        await card.locator('input[type=file]').setInputFiles({ name: 'results.json', mimeType: 'application/json', buffer: Buffer.from('{"suites":[{}]}') });
        await card.getByRole('button', { name: 'Upload' }).click();
        await expect(card.getByText(/3 cases with a result in 12 tests/)).toBeVisible();
        await expect(card.getByText('TC-ZZZ-999')).toBeVisible();
        expect(posted).toBe(1);
        if (shots) await page.screenshot({ path: `${shots}/results-${name}.png`, fullPage: true });

        // No horizontal page scroll.
        const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
        expect(overflow, 'no horizontal page scroll').toBeLessThanOrEqual(0);
      } finally {
        await cleanup(page);
      }
    });
  }
});
