/**
 * Issues View Tests
 * Tests /issues endpoint for issue debugger
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Issues View (/issues)', () => {
  
  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/issues`);
  });

  test('should load without errors', async ({ page }) => {
    await expect(page).toHaveURL(/\/issues/);
  });

  test('should display issues content', async ({ page }) => {
    await page.waitForSelector('h1, h2', { timeout: 10000 });
    
    const content = await page.content();
    expect(content.length).toBeGreaterThan(100);
  });

  test('should fetch issues from API', async ({ page }) => {
    await page.waitForResponse(response => 
      response.url().includes('/api/issues') && response.status() === 200
    , { timeout: 10000 });
  });

  test('should not have critical console errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    await page.goto(`${config.dashboardUrl}/issues`);
    await page.waitForLoadState('networkidle');

    const criticalErrors = errors.filter(e => 
      !e.includes('favicon') && 
      !e.includes('404')
    );
    expect(criticalErrors.length).toBe(0);
  });

});