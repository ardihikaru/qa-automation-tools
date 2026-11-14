/**
 * Health and Analytics API Tests
 * Tests basic system health and analytics endpoints
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Health & Analytics API', () => {
  
  test('GET /health - should return system status', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/health`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(body).toHaveProperty('status', 'ok');
    expect(body).toHaveProperty('database');
    expect(body).toHaveProperty('minimax');
  });

  test('GET /health - should show database connected', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/health`);
    const body = await response.json();
    
    expect(body.database).toBe('connected');
  });

  test('GET /health - should report the AI provider as healthy', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/health`);
    const body = await response.json();

    expect(['ai-manager', 'minimax']).toContain(body.ai_provider);
    expect(['ok', 'configured']).toContain(body.ai);
  });

});
