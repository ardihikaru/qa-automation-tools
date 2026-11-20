/**
 * Archive page and AI conversations tab (docs/designs/AI_CONVERSATIONS.md). Needs a login: set
 * QA_USERNAME and QA_PASSWORD. Read-only: it never archives or prunes.
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Archive page', () => {
  test.skip(!process.env.QA_USERNAME || !process.env.QA_PASSWORD, 'set QA_USERNAME and QA_PASSWORD');

  let role = '';

  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/login`);
    const inputs = page.locator('input');
    await inputs.nth(0).fill(process.env.QA_USERNAME!);
    await inputs.nth(1).fill(process.env.QA_PASSWORD!);
    const login = page.waitForResponse((r) => r.url().includes('/api/auth/login'));
    await page.click('button[type=submit]');
    role = (await (await login).json()).user?.role ?? '';
    await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  });

  test('the nav entry opens the archive with its prune button', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/products`);
    const list = page.waitForResponse((r) => r.url().includes('/api/conversations?') && r.status() === 200);
    await page.getByRole('link', { name: 'Archive' }).click();
    await list;
    await expect(page).toHaveURL(/\/archive/);
    const prune = page.getByRole('button', { name: 'Prune all archived' });
    await expect(prune).toBeVisible();
    if (role !== 'Admin') {
      await expect(prune).toBeDisabled();
      await expect(page.getByText(/Admins only/)).toBeVisible();
    }
  });

  test('the run history lists AI conversations', async ({ page }) => {
    const list = page.waitForResponse((r) => r.url().includes('/api/conversations?') && r.status() === 200);
    await page.goto(`${config.dashboardUrl}/history?tab=conversations`);
    await list;
    await expect(page.getByRole('tab', { name: 'AI conversations' })).toHaveAttribute('aria-selected', 'true');
  });
});
