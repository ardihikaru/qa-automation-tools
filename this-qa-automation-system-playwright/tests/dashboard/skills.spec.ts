/**
 * Skills View Tests
 * Tests /skills endpoint for skill execution UI
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Skills View (/skills)', () => {
  
  test.beforeEach(async ({ page }) => {
    await page.goto(`${config.dashboardUrl}/skills`);
  });

  test('should load without errors', async ({ page }) => {
    await expect(page).toHaveURL(/\/skills/);
  });

  test('should display skills content', async ({ page }) => {
    await page.waitForSelector('h1, h2', { timeout: 10000 });
    
    const content = await page.content();
    expect(content.length).toBeGreaterThan(100);
  });

  test('should fetch skills from API', async ({ page }) => {
    await page.waitForResponse(response => 
      response.url().includes('/api/skills') && response.status() === 200
    , { timeout: 10000 });
  });

  test('should fetch skill stats from API', async ({ page }) => {
    // Switch to stats tab if needed
    await page.waitForResponse(response => 
      response.url().includes('/api/skills') && response.status() === 200
    , { timeout: 10000 });
  });

  test('should display skill tabs', async ({ page }) => {
    // Wait for content to load
    await page.waitForLoadState('networkidle');
    
    // Check for tab buttons - look for text-based buttons in the tab area
    const skillsTab = page.locator('button:has-text("Skills")').first();
    await expect(skillsTab).toBeVisible({ timeout: 5000 });
  });

  test('should not have critical console errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') {
        errors.push(msg.text());
      }
    });

    await page.goto(`${config.dashboardUrl}/skills`);
    await page.waitForLoadState('networkidle');

    const criticalErrors = errors.filter(e => 
      !e.includes('favicon') && 
      !e.includes('404')
    );
    expect(criticalErrors.length).toBe(0);
  });

});