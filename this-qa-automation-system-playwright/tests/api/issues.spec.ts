/**
 * Issues API Tests
 * Tests issue management endpoints
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Issues API', () => {
  
  test('GET /api/issues - should return array of issues', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/issues`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(body).toHaveProperty('data');
    expect(Array.isArray(body.data)).toBe(true);
  });

  test('GET /api/issues - should include pagination info', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/issues`);
    const body = await response.json();
    
    expect(body).toHaveProperty('pagination');
    expect(body.pagination).toHaveProperty('page');
    expect(body.pagination).toHaveProperty('total');
    expect(body.pagination).toHaveProperty('total_pages');
  });

  test('GET /api/issues?is_fixed=true - should filter by fixed status', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/issues?is_fixed=true`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    // Should return filtered results
    expect(Array.isArray(body.data)).toBe(true);
  });

  test('GET /api/issues?category=frontend - should filter by category', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/issues?category=frontend`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body.data)).toBe(true);
  });

  test('GET /api/issues?search=test - should search issues', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/issues?search=test`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body.data)).toBe(true);
  });

  test('POST /api/issues - should create new issue', async ({ request }) => {
    const newIssue = {
      page: 'test-page',
      action: 'test-action',
      error_type: 'validation_error',
      error_text: 'Test error message',
      category: 'frontend',
      severity: 'medium'
    };
    
    const response = await request.post(`${config.backendUrl}/api/issues`, {
      data: newIssue
    });
    
    expect(response.status()).toBe(201);
    
    const body = await response.json();
    expect(body).toHaveProperty('id');
    expect(body.page).toBe('test-page');
  });

  test('POST /api/issues/status - should update issue fix status', async ({ request }) => {
    const updateData = {
      id: 1,
      is_fixed: true
    };
    
    const response = await request.post(`${config.backendUrl}/api/issues/status`, {
      data: updateData
    });
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(body.is_fixed).toBe(true);
  });

  test('POST /api/issues/notes - should add notes to issue', async ({ request }) => {
    const noteData = {
      id: 1,
      notes: 'This issue has been fixed'
    };
    
    const response = await request.post(`${config.backendUrl}/api/issues/notes`, {
      data: noteData
    });
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(body.notes).toBe('This issue has been fixed');
  });

  test('POST /api/issues - should handle invalid data gracefully', async ({ request }) => {
    const response = await request.post(`${config.backendUrl}/api/issues`, {
      data: {}
    });
    
    // Backend may accept empty issue or return error
    // Accept 201 (created) or 400 (bad request)
    expect([201, 400]).toContain(response.status());
  });

});