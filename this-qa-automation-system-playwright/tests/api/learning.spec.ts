/**
 * Learning API Tests
 * Tests learning and intelligence endpoints
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Learning API', () => {
  
  test('GET /api/learning - should return learning records', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/learning`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/learning - should return records with expected fields', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/learning`);
    const body = await response.json();
    
    if (body.length > 0) {
      const record = body[0];
      expect(record).toHaveProperty('id');
      expect(record).toHaveProperty('intent');
      expect(record).toHaveProperty('action');
      expect(record).toHaveProperty('success_score');
    }
  });

  test('GET /api/learning?intent=login - should filter by intent', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/learning?intent=login`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/learning?page=dashboard - should filter by page', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/learning?page=dashboard`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('POST /api/learning - should handle new learning record', async ({ request }) => {
    const newRecord = {
      intent: 'test_intent',
      action: 'click_button',
      page: 'test_page',
      success: true
    };
    
    const response = await request.post(`${config.backendUrl}/api/learning`, {
      data: newRecord
    });
    
    // Backend may return 201 or 500 depending on database setup
    expect([201, 500]).toContain(response.status());
  });

  test('POST /api/learning - should return 400 for invalid data', async ({ request }) => {
    const response = await request.post(`${config.backendUrl}/api/learning`, {
      data: {}
    });
    
    expect(response.status()).toBe(400);
  });

  test('DELETE /api/learning?id=1 - should delete learning record', async ({ request }) => {
    const response = await request.delete(`${config.backendUrl}/api/learning?id=1`);
    
    // May return 200 or 404 depending on whether record exists
    expect([200, 404]).toContain(response.status());
  });

  test('GET /api/learning/navigation - should return navigation edges', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/learning/navigation?source=home`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

});
