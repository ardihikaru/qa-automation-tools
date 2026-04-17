/**
 * Docs View Tests
 * Tests /docs endpoint for documentation viewer
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Docs View (/docs)', () => {
  
  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/docs`);
  });

  test('should load without errors', async ({ page }) => {
    await expect(page).toHaveURL(/\/docs/);
  });

  test('should display docs content', async ({ page }) => {
    // Wait for content to load
    await page.waitForSelector('h1, h2', { timeout: 10000 });
    
    // Check that page has some content
    const content = await page.content();
    expect(content.length).toBeGreaterThan(100);
  });

  test('should not have critical console errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    await page.goto(`${config.dashboardUrl}/docs`);
    await page.waitForLoadState('networkidle');

    // Filter out non-critical errors
    const criticalErrors = errors.filter(e => 
      !e.includes('favicon') && 
      !e.includes('404')
    );
    expect(criticalErrors.length).toBe(0);
  });

});