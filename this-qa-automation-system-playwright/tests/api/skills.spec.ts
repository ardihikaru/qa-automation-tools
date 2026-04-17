/**
 * Skills API Tests
 * Tests skill management and execution endpoints
 */

import { test, expect } from '@playwright/test';
import { config } from '../../config';

test.describe('Skills API', () => {
  
  test('GET /api/skills - should return array of skills', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/skills - should return active skills with expected fields', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills`);
    const body = await response.json();
    
    if (body.length > 0) {
      const skill = body[0];
      expect(skill).toHaveProperty('id');
      expect(skill).toHaveProperty('name');
      expect(skill).toHaveProperty('description');
      expect(skill).toHaveProperty('check_type');
      expect(skill).toHaveProperty('severity');
      expect(skill).toHaveProperty('is_active');
      expect(skill.is_active).toBe(true);
    }
  });

  test('GET /api/skills - should include all builtin skills', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills`);
    const body = await response.json();
    
    const skillNames = body.map((s: any) => s.name);
    expect(skillNames).toContain('max_length');
    expect(skillNames).toContain('required_field');
    expect(skillNames).toContain('data_persistence');
    expect(skillNames).toContain('multi_tab_save');
  });

  test('GET /api/skills/stats - should return skill performance statistics', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills/stats`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/skills/stats - should include pass rate', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills/stats`);
    const body = await response.json();
    
    if (body.length > 0) {
      const stat = body[0];
      expect(stat).toHaveProperty('total_runs');
      expect(stat).toHaveProperty('passed_runs');
      expect(stat).toHaveProperty('failed_runs');
      expect(stat).toHaveProperty('pass_rate');
    }
  });

  test('GET /api/skills/logs - should return execution logs', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills/logs`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/skills/logs?passed=true - should filter passed logs', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills/logs?passed=true`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/skills/logs?skill_id=1 - should filter by skill', async ({ request }) => {
    const response = await request.get(`${config.backendUrl}/api/skills/logs?skill_id=1`);
    
    expect(response.status()).toBe(200);
    
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/skills - should handle method not allowed', async ({ request }) => {
    const response = await request.post(`${config.backendUrl}/api/skills`, {
      data: { name: 'test' }
    });
    
    expect(response.status()).toBe(405);
  });

});