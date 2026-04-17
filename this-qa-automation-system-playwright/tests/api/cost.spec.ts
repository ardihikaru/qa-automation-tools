/**
 * Cost Analytics API Tests
 * Tests cost monitoring and optimization endpoints
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Cost Analytics API', () => {
  
  test('GET /api/cost/overview - should return cost overview', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/overview`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(body).toHaveProperty('total_requests');
    expect(body).toHaveProperty('total_tokens');
    expect(body).toHaveProperty('total_cost');
    expect(body).toHaveProperty('cache_hit_rate');
  });

  test('GET /api/cost/overview - should return numeric values', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/overview`);
    const body = await response.json();
    
    expect(typeof body.total_requests).toBe('number');
    expect(typeof body.total_tokens).toBe('number');
    expect(typeof body.total_cost).toBe('number');
    expect(typeof body.cache_hit_rate).toBe('number');
  });

  test('GET /api/cost/trend - should return cost trend data', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/trend`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/cost/trend - should include date and cost', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/trend`);
    const body = await response.json();
    
    if (body.length > 0) {
      const trend = body[0];
      expect(trend).toHaveProperty('date');
      expect(trend).toHaveProperty('cost');
    }
  });

  test('GET /api/cost/efficiency - should return cache hit/miss stats', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/efficiency`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(body).toHaveProperty('hit');
    expect(body).toHaveProperty('miss');
  });

  test('GET /api/cost/efficiency - should return numeric values', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/efficiency`);
    const body = await response.json();
    
    expect(typeof body.hit).toBe('number');
    expect(typeof body.miss).toBe('number');
  });

  test('GET /api/cost/by-feature - should return cost by feature', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/by-feature`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/cost/by-feature - should include feature and cost', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/cost/by-feature`);
    const body = await response.json();
    
    if (body.length > 0) {
      const feature = body[0];
      expect(feature).toHaveProperty('feature');
      expect(feature).toHaveProperty('cost');
    }
  });

  test('GET /api/cost/overview - should handle method not allowed', async ({ request }) => {
    const response = await request.post(`${config.backendUrl}/api/cost/overview`, {
      data: {}
    });
    
    expect(response.status()).toBe(405);
  });

});