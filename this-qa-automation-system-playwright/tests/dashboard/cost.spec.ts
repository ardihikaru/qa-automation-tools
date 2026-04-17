/**
 * Cost Analytics View Tests
 * Tests /cost endpoint for cost monitoring
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Cost Analytics View (/cost)', () => {
  
  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/cost`);
  });

  test('should load without errors', async ({ page }) => {
    await expect(page).toHaveURL(/\/cost/);
  });

  test('should display cost analytics content', async ({ page }) => {
    await page.waitForSelector('h1, h2', { timeout: 10000 });
    
    const content = await page.content();
    expect(content.length).toBeGreaterThan(100);
  });

  test('should fetch cost overview from API', async ({ page }) => {
    await page.waitForResponse(response => 
      response.url().includes('/api/cost/overview') && response.status() === 200
    , { timeout: 10000 });
  });

  test('should fetch cost trend from API', async ({ page }) => {
    await page.waitForResponse(response => 
      response.url().includes('/api/cost/trend') && response.status() === 200
    , { timeout: 10000 });
  });

  test('should fetch cost efficiency from API', async ({ page }) => {
    await page.waitForResponse(response => 
      response.url().includes('/api/cost/efficiency') && response.status() === 200
    , { timeout: 10000 });
  });

  test('should not have critical console errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    await page.goto(`${config.dashboardUrl}/cost`);
    await page.waitForLoadState('networkidle');

    // Filter out non-critical errors
    const criticalErrors = errors.filter(e => 
      !e.includes('favicon') && 
      !e.includes('404') &&
      !e.includes('net::ERR')
    );
    expect(criticalErrors.length).toBe(0);
  });

});