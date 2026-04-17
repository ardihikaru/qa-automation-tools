/**
 * Dashboard Navigation Tests
 * Tests dashboard navigation and basic page loads
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Dashboard Navigation', () => {
  
  test.beforeEach(async ({ page }) => {
    await page.goto(config.dashboardUrl);
  });

  test('should load dashboard home page', async ({ page }) => {
    await expect(page).toHaveTitle(/QA Automation Dashboard/);
  });

  test('should redirect to /docs by default', async ({ page }) => {
    await page.goto(config.dashboardUrl);
    await expect(page).toHaveURL(/\/docs/);
  });

  test('should navigate to /qa', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/qa`);
    await expect(page).toHaveURL(/\/qa/);
  });

  test('should navigate to /issues', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/issues`);
    await expect(page).toHaveURL(/\/issues/);
  });

  test('should navigate to /cost', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/cost`);
    await expect(page).toHaveURL(/\/cost/);
  });

  test('should navigate to /fixes', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/fixes`);
    await expect(page).toHaveURL(/\/fixes/);
  });

  test('should navigate to /skills', async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/skills`);
    await expect(page).toHaveURL(/\/skills/);
  });

  test('should not have console errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    await page.goto(config.dashboardUrl);
    await page.waitForLoadState('networkidle');

    // Filter out non-critical errors (favicon, 404s, etc.)
    const criticalErrors = errors.filter(e => 
      !e.includes('favicon') && 
      !e.includes('404') &&
      !e.includes('net::ERR')
    );
    expect(criticalErrors.length).toBe(0);
  });

});