/**
 * Layout width contract: every page's root element fills the content area.
 * Regression guard for views that cap their own width (e.g. `max-w-5xl`), which
 * leaves a wide screen mostly empty. Needs a login: set QA_USERNAME and QA_PASSWORD.
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

const ROUTES = ['docs', 'qa', 'issues', 'cost', 'fixes', 'skills', 'navigation', 'learning', 'products', 'settings'];

test.describe('Layout width (all pages fill the content area)', () => {
  test.skip(!process.env.QA_USERNAME || !process.env.QA_PASSWORD, 'set QA_USERNAME and QA_PASSWORD');
  test.use({ viewport: { width: 1920, height: 1080 } });

  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/login`);
    const inputs = page.locator('input');
    await inputs.nth(0).fill(process.env.QA_USERNAME!);
    await inputs.nth(1).fill(process.env.QA_PASSWORD!);
    await page.click('button[type=submit]');
    await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  });

  for (const route of ROUTES) {
    test(`/${route} root is as wide as the content area`, async ({ page }) => {
      await page.goto(`${config.dashboardUrl}/${route}`);
      await page.waitForSelector('main h1, main h2', { timeout: 10000 });
      const m = await page.evaluate(() => {
        const main = document.querySelector('main')!;
        const root = main.firstElementChild!.firstElementChild as HTMLElement; // PageTransition > view root
        const cs = getComputedStyle(main);
        const avail = main.clientWidth - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight);
        return {
          avail,
          root: root.getBoundingClientRect().width,
          overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        };
      });
      expect(Math.abs(m.root - m.avail), `root ${m.root}px vs available ${m.avail}px`).toBeLessThanOrEqual(1);
      expect(m.overflow, 'no horizontal page scroll').toBeLessThanOrEqual(0);
    });
  }
});
