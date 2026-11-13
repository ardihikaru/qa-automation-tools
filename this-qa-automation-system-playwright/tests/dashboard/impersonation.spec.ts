/**
 * Dashboard impersonation flow (docs/designs/IMPERSONATION_AND_SERVICE_ACCOUNT.md):
 * "Log in as another user" picker → amber banner → Return.
 *
 * Same env as tests/api/impersonation.spec.ts: E2E_IMPERSONATOR_USERNAME,
 * E2E_IMPERSONATOR_PASSWORD (Admin or Service), E2E_TARGET_USERNAME (any other
 * active user). Skipped when unset. Leaves one impersonation_audit row per run.
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword, impersonationTarget } = config;
const shots = process.env.E2E_SCREENSHOT_DIR;

test.describe('Dashboard impersonation', () => {
  test.skip(!impersonator || !impersonatorPassword || !impersonationTarget,
    'E2E_IMPERSONATOR_USERNAME / E2E_IMPERSONATOR_PASSWORD / E2E_TARGET_USERNAME not set');

  test('log in as another user, see the banner, return', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/login`);
    await page.fill('#login-username', impersonator);
    await page.fill('#login-password', impersonatorPassword);
    await page.click('button[type=submit]');
    await page.waitForURL((u) => !u.pathname.startsWith('/login'));

    try {
      // No banner while acting as yourself.
      await expect(page.getByRole('status').filter({ hasText: 'Viewing as' })).toHaveCount(0);

      await page.getByRole('button', { name: 'Log in as another user' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      await dialog.getByPlaceholder('Filter by username or name').fill(impersonationTarget);
      // The caller is hidden from the list.
      await expect(dialog.getByRole('button', { name: new RegExp(`^${impersonator}\\b`) })).toHaveCount(0);
      if (shots) await page.screenshot({ path: `${shots}/1-picker.png` });
      await dialog.getByRole('button', { name: new RegExp(`^${impersonationTarget}\\b`, 'i') }).first().click();

      // Full reload to "/" as the target, with the banner.
      const banner = page.getByRole('status').filter({ hasText: 'Viewing as' });
      await expect(banner).toBeVisible({ timeout: 15000 });
      await expect(banner).toContainText(new RegExp(impersonationTarget, 'i'));
      await expect(page.getByRole('button', { name: 'Log in as another user' })).toHaveCount(0);
      if (shots) await page.screenshot({ path: `${shots}/2-impersonating.png` });

      // Password change is not reachable while impersonating.
      await page.goto(`${config.dashboardUrl}/change-password`);
      await expect(page).not.toHaveURL(/change-password/);
      await expect(banner).toBeVisible();

      await banner.getByRole('button', { name: /Return to/ }).click();
      await expect(banner).toHaveCount(0, { timeout: 15000 });
      await expect(page.getByRole('button', { name: 'Log in as another user' })).toBeVisible();
      if (shots) await page.screenshot({ path: `${shots}/3-returned.png` });
    } finally {
      // Always leave the session clean, even after a failure.
      await page.request.post(`${config.dashboardUrl}/api/auth/impersonate/stop`, { headers: { 'Content-Type': 'application/json' }, data: {} });
      await page.request.post(`${config.dashboardUrl}/api/auth/logout`, { headers: { 'Content-Type': 'application/json' }, data: {} });
    }
  });
});
