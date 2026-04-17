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

  test('GET /health - should show minimax configured', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/health`);
    const body = await response.json();
    
    expect(body.minimax).toBe('configured');
  });

  test('GET /api/analytics - should return system analytics', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/analytics`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(body).toHaveProperty('flows');
    expect(body).toHaveProperty('issues');
    expect(body).toHaveProperty('skills');
    expect(body).toHaveProperty('total_records');
  });

  test('GET /api/analytics - should return numeric values', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/analytics`);
    const body = await response.json();
    
    expect(typeof body.flows).toBe('number');
    expect(typeof body.issues).toBe('number');
    expect(typeof body.skills).toBe('number');
  });

});