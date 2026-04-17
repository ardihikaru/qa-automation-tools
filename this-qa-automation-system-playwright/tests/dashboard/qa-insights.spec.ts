/**
 * QA Insights View Tests
 * Tests /qa endpoint for main dashboard overview
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('QA Insights View (/qa)', () => {
  
  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/qa`);
  });

  test('should load without errors', async ({ page }) => {
    await expect(page).toHaveURL(/\/qa/);
  });

  test('should display QA insights content', async ({ page }) => {
    // Wait for content to load
    await page.waitForSelector('h1, h2', { timeout: 10000 });
    
    // Check that page has some content
    const content = await page.content();
    expect(content.length).toBeGreaterThan(100);
  });

  test('should fetch analytics from API', async ({ page }) => {
    // Wait for API calls to complete
    await page.waitForResponse(response => 
      response.url().includes('/api/analytics') && response.status() === 200
    , { timeout: 10000 });
  });

  test('should not have critical console errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    await page.goto(`${config.dashboardUrl}/qa`);
    await page.waitForLoadState('networkidle');

    const criticalErrors = errors.filter(e => 
      !e.includes('favicon') && 
      !e.includes('404')
    );
    expect(criticalErrors.length).toBe(0);
  });

});