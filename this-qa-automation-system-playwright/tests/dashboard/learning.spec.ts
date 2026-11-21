/**
 * The /learning page (docs/designs/LEARNING_LOOP.md): product filter, tiles, tabs, and the
 * approval actions of a policy awaiting approval.
 *
 * Read-only against a deployed dashboard: logs in with QA_USERNAME / QA_PASSWORD. The learning
 * API is answered by the browser (page.route) with fixed data, so the test does not depend on
 * what the server has learned and never writes; the approval POST is intercepted too.
 */

import { test, expect, Page } from '@playwright/test';
import { config } from '../../config';

const shots = process.env.E2E_SCREENSHOT_DIR;
const last = <T,>(xs: T[]): T | undefined => xs[xs.length - 1];

const finding = {
  id: 41,
  product_id: 3,
  product_name: 'SUT',
  kind: 'review_changes',
  status: 'new',
  title: 'Reviewer asked for changes: the URL doubles /api/v1',
  detail: 'SUT_VENDOR_BACKEND_URL already ends in /api/v1; the spec appended it again.',
  occurrences: 2,
  first_seen_at: '2026-11-20T08:00:00Z',
  last_seen_at: '2026-11-20T09:00:00Z',
  updated_at: '2026-11-20T09:00:00Z',
  proposal_job_id: '',
  created_by: 'system',
  status_note: '',
  sources: [{ job_id: 'job-1', case_code: 'TC-VSYS-003' }],
};
const proposal = {
  id: 'job-77',
  product_id: 3,
  product_name: 'SUT',
  state: 'awaiting_approval',
  status: 'running',
  created_at: '2026-11-21T08:00:00Z',
  completed_at: null,
  triggered_by: 'u1',
  finding_ids: [41],
  title: 'Build vendor URLs from the registry base URL',
  slug: 'vendor-url-base',
  file_path: '.claude/rules/learned-vendor-url-base.md',
  policy: 'Use restServices baseURL; never append /api/v1 again.',
  rationale: 'Seen twice in review.',
  covered_by: '',
  content: '# Learned policy: Build vendor URLs from the registry base URL\n\nUse restServices baseURL; never append /api/v1 again.\n',
  branch: 'qa-auto/learn-job-77',
  pr_url: '',
  pr_state: '',
  merge_commit: '',
  merged_at: '',
  pr_checked_at: '',
  in_repo: false,
  error: '',
};

async function mockLearning(page: Page) {
  const approvals: string[] = [];
  await page.route('**/api/products', (r) =>
    r.fulfill({ json: { products: [{ id: 3, name: 'SUT', repos: [{ is_automation: true }] }, { id: 9, name: 'Other', repos: [] }], pool_enabled: true } }),
  );
  await page.route('**/api/learning/findings**', (r) =>
    r.fulfill({ json: { items: [finding], total: 1, counts: { new: 1, proposed: 0, learned: 0, covered: 0, dismissed: 0 } } }),
  );
  await page.route('**/api/learning/proposals**', (r) =>
    r.fulfill({
      json: {
        items: [proposal],
        total: 1,
        counts: { drafting: 0, awaiting_approval: 1, publishing: 0, pr_open: 0, learned: 0, covered: 0, rejected: 0, closed: 0, failed: 0 },
      },
    }),
  );
  await page.route('**/api/learning/timeline**', (r) =>
    r.fulfill({
      json: {
        items: [
          { at: '2026-11-21T08:00:00Z', kind: 'proposal_created', product_id: 3, product_name: 'SUT', job_id: 'job-77', title: proposal.title, state: 'awaiting_approval' },
          { at: '2026-11-20T08:00:00Z', kind: 'finding_new', product_id: 3, product_name: 'SUT', finding_id: 41, finding_kind: 'review_changes', title: finding.title, case_codes: ['TC-VSYS-003'], occurrences: 2 },
        ],
      },
    }),
  );
  await page.route('**/api/jobs/job-77/approval', (r) => {
    approvals.push(r.request().postData() ?? '');
    return r.fulfill({ json: { status: 'approved' } });
  });
  return approvals;
}

test.describe('Learning page', () => {
  test.skip(!process.env.QA_USERNAME || !process.env.QA_PASSWORD, 'set QA_USERNAME and QA_PASSWORD');

  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/login`);
    const inputs = page.locator('input');
    await inputs.nth(0).fill(process.env.QA_USERNAME!);
    await inputs.nth(1).fill(process.env.QA_PASSWORD!);
    await page.click('button[type=submit]');
    await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  });

  test('findings and the timeline page 10 at a time; a filter goes back to page 1', async ({ page }) => {
    await mockLearning(page);
    // 23 findings: three pages. The mock answers the requested slice so the pager can be followed.
    const seen: URLSearchParams[] = [];
    const all = Array.from({ length: 23 }, (_, i) => ({ ...finding, id: 100 + i, title: `Finding number ${i + 1}` }));
    await page.route('**/api/learning/findings**', (r) => {
      const u = new URL(r.request().url()).searchParams;
      seen.push(u);
      const off = Number(u.get('offset') ?? 0);
      const lim = Number(u.get('limit') ?? 50);
      return r.fulfill({ json: { items: all.slice(off, off + lim), total: all.length, counts: { new: 23, proposed: 0, learned: 0, covered: 0, dismissed: 0 } } });
    });
    await page.route('**/api/learning/timeline**', (r) => {
      const u = new URL(r.request().url()).searchParams;
      const items = Array.from({ length: 12 }, (_, i) => ({ at: '2026-11-20T08:00:00Z', kind: 'finding_new', product_id: 3, product_name: 'SUT', finding_id: i, title: `Event ${i + 1}` }));
      const off = Number(u.get('offset') ?? 0);
      return r.fulfill({ json: { items: items.slice(off, off + Number(u.get('limit'))), total: items.length } });
    });

    await page.goto(`${config.dashboardUrl}/learning`);
    const pager = page.getByRole('navigation', { name: 'Pagination' });
    await expect(pager).toContainText('1–10 of 12');

    await page.getByRole('tab', { name: /Findings/ }).click();
    await expect(page.getByText('Finding number 10', { exact: true })).toBeVisible();
    await expect(page.getByText('Finding number 11', { exact: true })).toHaveCount(0);
    await expect(pager).toContainText('1–10 of 23');
    expect(last(seen)?.get('limit')).toBe('10');

    await pager.getByRole('button', { name: 'Next page' }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.getByText('Finding number 11', { exact: true })).toBeVisible();
    await expect(pager).toContainText('11–20 of 23');
    expect(last(seen)?.get('offset')).toBe('10');

    // A filter change resets to page 1.
    await page.getByLabel('Finding status').selectOption('new');
    await expect(page).not.toHaveURL(/page=2/);
    await expect.poll(() => last(seen)?.get('offset') ?? '0').toBe('0');
    await expect(pager).toContainText('1–10 of 23');
  });

  for (const [name, viewport] of [['desktop', { width: 1440, height: 900 }], ['phone', { width: 375, height: 800 }]] as const) {
    test(`timeline, product filter, findings and policies (${name})`, async ({ page }) => {
      await page.setViewportSize(viewport);
      const approvals = await mockLearning(page);
      await page.goto(`${config.dashboardUrl}/learning`);
      await expect(page.getByRole('heading', { name: 'Learning', exact: true }).first()).toBeVisible();

      // The product filter exists and the choice lands in the URL.
      const product = page.getByLabel('Product');
      await expect(product).toBeVisible();
      await expect(product.locator('option', { hasText: 'All products' })).toHaveCount(1);
      // The default product is the first one with an automation repo.
      await expect(page).toHaveURL(/product=3/);
      await expect(product).toHaveValue('3');

      // Timeline is the default tab.
      await expect(page.getByRole('list', { name: 'Learning timeline' })).toContainText('Policy proposal started');
      await expect(page.getByText('New finding').first()).toBeVisible();
      if (shots) await page.screenshot({ path: `${shots}/learning-timeline-${name}.png`, fullPage: true });

      // Findings tab: the finding, its evidence link, and the draft button.
      await page.getByRole('tab', { name: /Findings/ }).click();
      await expect(page).toHaveURL(/tab=findings/);
      await expect(page.getByText(finding.title)).toBeVisible();
      await expect(page.getByText('seen 2×')).toBeVisible();

      // Policies tab: the awaiting policy shows its file and the approval actions.
      await page.getByRole('tab', { name: /Policies/ }).click();
      await expect(page.getByText('.claude/rules/learned-vendor-url-base.md').first()).toBeVisible();
      await expect(page.getByLabel('Policy file')).toContainText('never append /api/v1 again');
      if (shots) await page.screenshot({ path: `${shots}/learning-policies-${name}.png`, fullPage: true });
      const approve = page.getByRole('button', { name: 'Approve and open PR' });
      if (await approve.count()) {
        await approve.click();
        await expect.poll(() => approvals.length).toBe(1);
        expect(approvals[0]).toContain('"decision":"approve"');
      }

      // No horizontal page scroll at this width.
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow).toBeLessThanOrEqual(0);
    });
  }
});
