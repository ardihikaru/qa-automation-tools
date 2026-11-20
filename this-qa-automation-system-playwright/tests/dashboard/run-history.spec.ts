/**
 * Run history page (/history, docs/designs/RUN_HISTORY.md). Successor of the removed QA Insights
 * page (/qa, now a redirect). Needs a login: set QA_USERNAME and QA_PASSWORD. Read-only.
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Run history page', () => {
  test.skip(!process.env.QA_USERNAME || !process.env.QA_PASSWORD, 'set QA_USERNAME and QA_PASSWORD');

  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/login`);
    const inputs = page.locator('input');
    await inputs.nth(0).fill(process.env.QA_USERNAME!);
    await inputs.nth(1).fill(process.env.QA_PASSWORD!);
    await page.click('button[type=submit]');
    await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  });

  test('/qa redirects to /history and the nav entry is there', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/qa`);
    await expect(page).toHaveURL(/\/history/);
    await expect(page.getByRole('link', { name: 'Run history' })).toBeVisible();
  });

  test('the builds list loads from /api/history/builds', async ({ page }) => {
    const res = page.waitForResponse((r) => r.url().includes('/api/history/builds') && r.status() === 200);
    await page.goto(`${config.dashboardUrl}/history`);
    await res;
    await expect(page.getByRole('tab', { name: 'Builds' })).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByText(/No builds match these filters|waiting for approval/).first()).toBeVisible();
  });

  test('tabs switch and the URL keeps the tab', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/history`);
    const runs = page.waitForResponse((r) => r.url().includes('/api/history/runs') && r.status() === 200);
    await page.getByRole('tab', { name: 'Playwright runs' }).click();
    await runs;
    await expect(page).toHaveURL(/tab=runs/);
    await page.reload();
    await expect(page.getByRole('tab', { name: 'Playwright runs' })).toHaveAttribute('aria-selected', 'true');
    const syncs = page.waitForResponse((r) => r.url().includes('/api/history/syncs') && r.status() === 200);
    await page.getByRole('tab', { name: 'Knowledge syncs' }).click();
    await syncs;
    await expect(page).toHaveURL(/tab=syncs/);
  });

  test('no console errors after sign-in', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
    await page.goto(`${config.dashboardUrl}/history`);
    await page.waitForLoadState('networkidle');
    expect(errors.filter((e) => !e.includes('favicon'))).toEqual([]);
  });
});
