import { Page } from 'playwright';
import { Credential } from '../config';

export interface LoginResult {
  ok: boolean;
  url: string;
  error?: string;
}

/**
 * UI login to the SUT frontend (ported from playwright-qa random.ts loginAs).
 * SUT uses input[name=username] (not email). Success means we left /auth/login.
 */
export async function loginAs(page: Page, baseUrl: string, cred: Credential): Promise<LoginResult> {
  try {
    await page.goto(`${baseUrl}/auth/login`, { timeout: 30_000, waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[name="username"]', { timeout: 15_000 });
    await page.fill('input[name="username"]', cred.username);
    await page.fill('input[name="password"]', cred.password);
    await page.locator('button[type="submit"], button:has-text("Login")').first().click();
    await page
      .waitForURL((u) => !u.pathname.includes('/auth/login'), { timeout: 20_000 })
      .catch(() => undefined);
    await page.waitForLoadState('networkidle', { timeout: 10_000 }).catch(() => undefined);
    const url = page.url();
    if (new URL(url).pathname.includes('/auth/login')) {
      return { ok: false, url, error: 'still on /auth/login after submit (bad credentials or slow staging)' };
    }
    return { ok: true, url };
  } catch (err) {
    return { ok: false, url: page.url(), error: err instanceof Error ? err.message.split('\n')[0] : String(err) };
  }
}
