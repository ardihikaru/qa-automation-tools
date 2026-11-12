/**
 * qa_run: per role, log in, take `max_steps` random same-origin clicks
 * (ported from playwright-qa QARunner.randomStep), run built-in skills on each
 * newly visited page, and collect page errors and HTTP 5xx responses.
 */
import { Page } from 'playwright';
import { throwIfAborted } from '../browser';
import { loginAs } from '../sut/login';
import { runSkills, Severity } from '../skills/builtin';
import { RunParams } from '../types';
import { errMsg, newRoleContext, RunContext } from './context';

export interface QaIssue {
  role: string;
  url: string;
  type: 'skill' | 'page_error' | 'http_5xx' | 'login';
  skill?: string;
  severity: Severity;
  title: string;
  message: string;
  elements?: string[];
}

interface Step {
  n: number;
  from: string;
  to: string;
  target: string;
  ok: boolean;
  error?: string;
}

interface RoleReport {
  role: string;
  logged_in: boolean;
  login_error?: string;
  steps: Step[];
  pages_visited: string[];
  error?: string;
}

const SKIP_TEXT = /(log\s?out|sign\s?out|keluar|hapus|delete|remove|batal|cancel)/i;
const MAX_ISSUES_PER_ROLE = 200;

const pathOf = (u: string) => {
  try {
    const x = new URL(u);
    return `${x.pathname}${x.hash}`;
  } catch {
    return u;
  }
};

/** Pick a random visible, same-origin, non-destructive clickable element. */
async function randomClick(page: Page, origin: string): Promise<{ target: string; clicked: boolean }> {
  const candidates = page.locator('a[href], button:not([disabled]), [role="button"], [role="menuitem"]');
  const total = Math.min(await candidates.count(), 300);
  if (total === 0) return { target: '(no clickable elements)', clicked: false };

  let blocked = 0;
  for (let attempt = 0; attempt < 12; attempt++) {
    // A modal/drawer opened by an earlier click covers the page: dismiss it.
    if (blocked === 3) await page.keyboard.press('Escape').catch(() => undefined);
    const el = candidates.nth(Math.floor(Math.random() * total));
    const info = await el
      .evaluate((node) => {
        const r = (node as HTMLElement).getBoundingClientRect();
        return {
          visible: r.width > 0 && r.height > 0,
          text: ((node as HTMLElement).innerText || node.getAttribute('aria-label') || node.getAttribute('title') || '')
            .trim()
            .slice(0, 80),
          href: node instanceof HTMLAnchorElement ? node.href : '',
          type: node.getAttribute('type') || '',
        };
      })
      .catch(() => null);
    if (!info || !info.visible) continue;
    if (SKIP_TEXT.test(info.text)) continue;
    if (info.type === 'submit') continue; // never submit forms during random exploration
    if (info.href && !info.href.startsWith(origin)) continue;
    const target = info.text || info.href || '(unnamed element)';
    // Trial click = actionability check only (visible, stable, not covered).
    const actionable = await el.click({ trial: true, timeout: 1_500 }).then(
      () => true,
      () => false,
    );
    if (!actionable) {
      blocked++;
      continue;
    }
    try {
      await el.click({ timeout: 5_000 });
      await page.waitForLoadState('domcontentloaded', { timeout: 10_000 }).catch(() => undefined);
      await page.waitForTimeout(700);
      return { target, clicked: true };
    } catch (err) {
      return { target: `${target} (${errMsg(err)})`, clicked: false };
    }
  }
  return { target: '(no eligible element)', clicked: false };
}

export async function qaRun(ctx: RunContext, params: RunParams): Promise<Record<string, unknown>> {
  const { cfg, browser, signal } = ctx;
  const origin = new URL(cfg.stagingUrl).origin;
  const roles: RoleReport[] = [];
  const issues: QaIssue[] = [];
  const seen = new Set<string>(); // dedupe issues by role|url|type|skill

  const addIssue = (i: QaIssue, roleIssues: { n: number }) => {
    const key = `${i.role}|${pathOf(i.url)}|${i.type}|${i.skill ?? i.title}`;
    if (seen.has(key) || roleIssues.n >= MAX_ISSUES_PER_ROLE) return;
    seen.add(key);
    roleIssues.n++;
    issues.push(i);
  };

  for (const role of params.roles) {
    throwIfAborted(signal);
    const report: RoleReport = { role, logged_in: false, steps: [], pages_visited: [] };
    roles.push(report);
    const roleIssues = { n: 0 };
    const context = await newRoleContext(browser);
    try {
      const page = await context.newPage();
      page.on('pageerror', (e) =>
        addIssue(
          { role, url: page.url(), type: 'page_error', severity: 'medium', title: 'Uncaught JavaScript error', message: errMsg(e) },
          roleIssues,
        ),
      );
      page.on('response', (res) => {
        if (res.status() >= 500 && res.url().startsWith('http')) {
          addIssue(
            {
              role,
              url: page.url(),
              type: 'http_5xx',
              severity: 'high',
              title: `HTTP ${res.status()} from ${pathOf(res.url())}`,
              message: `${res.request().method()} ${res.url()} returned ${res.status()}`,
            },
            roleIssues,
          );
        }
      });

      const login = await loginAs(page, cfg.stagingUrl, cfg.credentials[role]);
      report.logged_in = login.ok;
      if (!login.ok) {
        report.login_error = login.error;
        addIssue(
          { role, url: login.url, type: 'login', severity: 'high', title: `Login failed for role ${role}`, message: login.error ?? 'login failed' },
          roleIssues,
        );
        continue;
      }

      const visited = new Set<string>();
      const inspect = async () => {
        const p = pathOf(page.url());
        if (visited.has(p)) return;
        visited.add(p);
        report.pages_visited.push(p);
        for (const r of await runSkills(page, params.interactive_skills)) {
          if (!r.passed) {
            addIssue(
              {
                role,
                url: page.url(),
                type: 'skill',
                skill: r.skill,
                severity: r.severity,
                title: `${r.skill} on ${p}`,
                message: r.message,
                elements: r.elements,
              },
              roleIssues,
            );
          }
        }
      };

      await inspect();
      for (let n = 1; n <= params.max_steps; n++) {
        throwIfAborted(signal);
        const from = page.url();
        const { target, clicked } = await randomClick(page, origin);
        // Left staging (new tab target, redirect, external link)? Go back home.
        if (!page.url().startsWith(origin)) {
          await page.goto(login.url, { waitUntil: 'domcontentloaded', timeout: 30_000 }).catch(() => undefined);
        }
        // Got logged out? Log back in once per step.
        if (pathOf(page.url()).includes('/auth/login')) {
          const again = await loginAs(page, cfg.stagingUrl, cfg.credentials[role]);
          if (!again.ok) {
            report.error = `session lost at step ${n}: ${again.error}`;
            break;
          }
        }
        report.steps.push({ n, from: pathOf(from), to: pathOf(page.url()), target, ok: clicked });
        await inspect();
      }
    } catch (err) {
      if (signal.aborted) throw err;
      report.error = errMsg(err);
    } finally {
      await context.close().catch(() => undefined);
    }
  }

  const bySeverity = { high: 0, medium: 0, low: 0 };
  for (const i of issues) bySeverity[i.severity]++;
  return {
    target: origin,
    roles,
    issues,
    summary: {
      roles: roles.length,
      logged_in: roles.filter((r) => r.logged_in).length,
      steps: roles.reduce((a, r) => a + r.steps.length, 0),
      pages: roles.reduce((a, r) => a + r.pages_visited.length, 0),
      issues: issues.length,
      by_severity: bySeverity,
    },
  };
}
