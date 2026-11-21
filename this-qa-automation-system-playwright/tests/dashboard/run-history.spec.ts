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

  // Every tab asks for 10 rows a page and has a Product column (folded into the row on a phone).
  for (const [tab, api, label] of [
    ['builds', '/api/history/builds', 'Builds'],
    ['runs', '/api/history/runs', 'Playwright runs'],
    ['syncs', '/api/history/syncs', 'Knowledge syncs'],
    ['conversations', '/api/conversations?', 'AI conversations'],
  ] as const) {
    test(`${label}: 10 per page and a Product column`, async ({ page }) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      const res = page.waitForResponse((r) => r.url().includes(api) && r.status() === 200);
      await page.goto(`${config.dashboardUrl}/history?tab=${tab}`);
      const r = await res;
      expect(new URL(r.url()).searchParams.get('page_size')).toBe('10');
      const body = (await r.json()) as { total: number; items: unknown[] };
      expect(body.items.length).toBeLessThanOrEqual(10);
      if (body.total === 0) return; // an empty list has no table to check
      const product = page.getByRole('columnheader', { name: 'Product', exact: true });
      await expect(product).toBeVisible();
      const pager = page.getByRole('navigation', { name: 'Pagination' });
      await expect(pager).toContainText(`1–${Math.min(10, body.total)} of ${body.total.toLocaleString()}`);
      await expect(pager.getByRole('combobox')).toHaveValue('10');
      if (body.total > 10) {
        const next = page.waitForResponse((x) => x.url().includes(api) && x.status() === 200);
        await pager.getByRole('button', { name: 'Next page' }).click();
        expect(new URL((await next).url()).searchParams.get('page')).toBe('2');
        await expect(page).toHaveURL(/page=2/);
        await expect(pager).toContainText(`11–${Math.min(20, body.total)} of`);
      }
      // Phone width: the column is hidden and the page does not scroll sideways.
      await page.setViewportSize({ width: 375, height: 800 });
      await expect(product).toBeHidden();
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow).toBeLessThanOrEqual(0);
    });
  }

  test('a filter change goes back to page 1', async ({ page }) => {
    const first = page.waitForResponse((r) => r.url().includes('/api/history/builds') && r.status() === 200);
    await page.goto(`${config.dashboardUrl}/history?tab=builds&page=2`);
    await first;
    const res = page.waitForResponse((r) => r.url().includes('/api/history/builds') && r.status() === 200);
    await page.getByLabel('Status').selectOption('completed');
    expect(new URL((await res).url()).searchParams.get('page')).toBeNull();
    await expect(page).not.toHaveURL(/page=2/);
  });

  test('no console errors after sign-in', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
    await page.goto(`${config.dashboardUrl}/history`);
    await page.waitForLoadState('networkidle');
    expect(errors.filter((e) => !e.includes('favicon'))).toEqual([]);
  });
});
