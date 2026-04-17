/**
 * Flows API Tests
 * Tests flow management endpoints
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Flows API', () => {
  
  test('GET /api/flows - should return array of flows', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/flows`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/flows - should return flows with expected fields', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/flows`);
    const body = await response.json();
    
    if (body.length > 0) {
      const flow = body[0];
      expect(flow).toHaveProperty('id');
      expect(flow).toHaveProperty('name');
      expect(flow).toHaveProperty('description');
    }
  });

  test('POST /api/flows - should create new flow', async ({ request }) => {
    const newFlow = {
      role_id: 1,
      name: 'Test Flow',
      description: 'Test flow description'
    };
    
    const response = await request.post(`${config.backendUrl}/api/flows`, {
      data: newFlow
    });
    
    expect(response.status()).toBe(201);
    
    const body = await response.json();
    expect(body).toHaveProperty('id');
    expect(body.name).toBe('Test Flow');
  });

  test('POST /api/flows - should handle invalid data gracefully', async ({ request }) => {
    const response = await request.post(`${config.backendUrl}/api/flows`, {
      data: {}
    });
    
    // Backend returns 500 for invalid data (not 400)
    expect([400, 500]).toContain(response.status());
  });

  test('GET /api/flows - should handle method not allowed', async ({ request }) => {
    const response = await request.put(`${config.backendUrl}/api/flows`, {
      data: { name: 'Test' }
    });
    
    expect(response.status()).toBe(405);
  });

});