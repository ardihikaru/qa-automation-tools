/**
 * The /navigation page (docs/designs/NAVIGATION_MAP.md section 7). Read-only against production:
 * logs in as the impersonator, opens the SUT map and one route's drill-down. Skipped without
 * E2E_IMPERSONATOR_USERNAME / _PASSWORD; the tree checks skip until the map is built.
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

const { impersonator, impersonatorPassword } = config;

test.describe('Navigation map page', () => {
  test.skip(!impersonator || !impersonatorPassword, 'E2E_IMPERSONATOR_USERNAME / E2E_IMPERSONATOR_PASSWORD not set');

  for (const [name, viewport] of [['desktop', { width: 1440, height: 900 }], ['phone', { width: 375, height: 800 }]] as const) {
    test(`map, filters and drill-down render (${name})`, async ({ page }) => {
      await page.setViewportSize(viewport);
      await page.goto(`${config.dashboardUrl}/login`);
      await page.fill('#login-username', impersonator);
      await page.fill('#login-password', impersonatorPassword);
      await page.click('button[type=submit]');
      await page.waitForURL((u) => !u.pathname.startsWith('/login'));

      const resp = page.waitForResponse((r) => /\/api\/products\/\d+\/navigation\?/.test(r.url()) && r.request().method() === 'GET');
      await page.goto(`${config.dashboardUrl}/navigation?surface=api&app=sut-vendor-backend`);
      expect((await resp).status()).toBe(200);
      const noBuild = page.getByText('No navigation map for this product yet');
      const header = page.getByRole('heading', { name: 'sut-vendor-backend' });
      await expect(noBuild.or(header)).toBeVisible({ timeout: 15000 });
      test.skip(await noBuild.isVisible(), 'no navigation build yet');

      await expect(page.getByRole('button', { name: /Failing \d+/ })).toBeVisible();
      await page.getByRole('button', { name: /service-status/ }).first().click();
      await expect(page).toHaveURL(/route=[0-9a-f]{40}/);
      const panel = page.getByRole('complementary', { name: 'Route details' });
      await expect(panel).toBeVisible();
      await expect(panel.getByRole('link', { name: 'TC-VSYS-001', exact: true })).toBeVisible({ timeout: 15000 });
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      expect(overflow).toBeLessThanOrEqual(0);
    });
  }
});
