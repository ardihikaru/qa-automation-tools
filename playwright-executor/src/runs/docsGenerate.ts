/**
 * docs_generate: per role, log in and replay the documentation flows
 * (ported from playwright-docs recordFlow), capturing a screenshot and the
 * page title/heading after every step. The backend turns the result into
 * Markdown through ai-manager/claude-v3; the executor itself makes no AI calls.
 */
import * as fs from 'fs';
import * as path from 'path';
import { Page } from 'playwright';
import { throwIfAborted } from '../browser';
import { FlowStep, flowsForRole } from '../flows/flows';
import { loginAs } from '../hisv3/login';
import { RunParams } from '../types';
import { errMsg, newRoleContext, RunContext } from './context';

interface StepRecord {
  n: number;
  action: FlowStep['action'];
  target?: string;
  ok: boolean;
  url: string;
  title: string;
  heading: string;
  screenshot?: string;
  error?: string;
}

const safe = (s: string) => s.replace(/[^a-zA-Z0-9._-]+/g, '_');
const NOT_FOUND = /^\/(404|not-found)\/?$/;
const SKIP_PATH = /(logout|sign-?out|keluar|delete|hapus)/i;

/**
 * Same-origin page paths linked from the navigation of the current page
 * (sidebar/nav/menus), in document order. Collapsed menus are expanded first
 * by clicking their toggles, without following any link.
 */
async function discoverMenu(page: Page, baseUrl: string): Promise<string[]> {
  const origin = new URL(baseUrl).origin;
  const NAV = 'nav, aside, [role="navigation"], [role="menu"], [class*="sidebar" i], [class*="menu" i]';
  // Expand collapsible groups (buttons/summary inside nav that are not links).
  const toggles = page.locator(
    `:is(${NAV}) :is(button, summary, [aria-expanded="false"]):not(a):not([type="submit"])`,
  );
  const n = Math.min(await toggles.count().catch(() => 0), 40);
  for (let i = 0; i < n; i++) {
    const t = toggles.nth(i);
    if ((await t.getAttribute('aria-expanded').catch(() => null)) === 'true') continue;
    const label = await t.innerText({ timeout: 500 }).catch(() => '');
    if (/(log\s?out|sign\s?out|keluar|hapus|delete|profil|profile|akun|account|notif)/i.test(label)) continue;
    await t.click({ trial: true, timeout: 500 }).then(
      () => t.click({ timeout: 1_000 }).catch(() => undefined),
      () => undefined,
    );
  }
  await page.waitForTimeout(300);
  const hrefs = await page
    .$$eval(`:is(${NAV}) a[href]`, (as) => as.map((a) => (a as HTMLAnchorElement).href))
    .catch(() => [] as string[]);
  const out: string[] = [];
  for (const h of hrefs) {
    if (!h.startsWith(origin)) continue;
    const p = new URL(h).pathname;
    if (p === '/' || SKIP_PATH.test(p) || NOT_FOUND.test(p) || p.includes('/auth/') || out.includes(p)) continue;
    out.push(p);
  }
  return out;
}

async function pageFacts(page: Page): Promise<{ title: string; heading: string }> {
  const title = await page.title().catch(() => '');
  const heading = await page
    .locator('h1, h2, [role="heading"]')
    .first()
    .innerText({ timeout: 1_000 })
    .catch(() => '');
  return { title: title.slice(0, 200), heading: heading.trim().slice(0, 200) };
}

async function doStep(page: Page, base: string, step: FlowStep): Promise<void> {
  switch (step.action) {
    case 'navigate':
      await page.goto(base + (step.selector ?? '/'), { waitUntil: 'domcontentloaded', timeout: 30_000 });
      return;
    case 'click':
      if (step.selector) await page.locator(step.selector).first().click({ timeout: 10_000 });
      return;
    case 'type':
      if (step.selector) await page.locator(step.selector).first().fill(step.value ?? '', { timeout: 10_000 });
      return;
    case 'select':
      if (step.selector && step.value) await page.selectOption(step.selector, step.value, { timeout: 10_000 });
      return;
    case 'wait':
      await page.waitForTimeout(Math.min(step.wait ?? 1000, 10_000));
      return;
  }
}

export async function docsGenerate(ctx: RunContext, params: RunParams): Promise<Record<string, unknown>> {
  const { cfg, browser, signal, outDir } = ctx;
  const flowsOut: Record<string, unknown>[] = [];
  const roleErrors: Record<string, string> = {};
  let screenshots = 0;

  for (const role of params.roles) {
    throwIfAborted(signal);
    const context = await newRoleContext(browser);
    try {
      const page = await context.newPage();
      const login = await loginAs(page, cfg.stagingUrl, cfg.credentials[role]);
      if (!login.ok) {
        roleErrors[role] = login.error ?? 'login failed';
        continue;
      }
      // Curated flows first, then pages discovered from this role's real menu
      // (HISv3 routes differ per role/version; curated paths can go stale).
      const flows = flowsForRole(role, params.flows);
      if (params.flows.length === 0) {
        const known = new Set(flows.flatMap((f) => f.steps.filter((s) => s.action === 'navigate').map((s) => s.selector)));
        const discovered = (await discoverMenu(page, cfg.stagingUrl)).filter((p) => !known.has(p));
        for (const p of discovered.slice(0, params.max_steps)) {
          flows.push({
            id: `menu${p.replace(/[^a-zA-Z0-9]+/g, '-').replace(/-+$/, '')}`,
            name: `Menu: ${p}`,
            description: `Page reached from the ${role} navigation menu`,
            steps: [
              { action: 'navigate', selector: p },
              { action: 'wait', wait: 2000 },
            ],
          });
        }
      }
      for (const flow of flows) {
        throwIfAborted(signal);
        const dir = path.join(outDir, safe(role), safe(flow.id));
        fs.mkdirSync(dir, { recursive: true });
        const steps: StepRecord[] = [];
        for (let i = 0; i < flow.steps.length; i++) {
          throwIfAborted(signal);
          const step = flow.steps[i];
          const rec: StepRecord = { n: i + 1, action: step.action, target: step.selector, ok: true, url: '', title: '', heading: '' };
          try {
            await doStep(page, cfg.stagingUrl, step);
            await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => undefined);
          } catch (err) {
            if (signal.aborted) throw err;
            rec.ok = false;
            rec.error = errMsg(err);
          }
          rec.url = new URL(page.url()).pathname;
          Object.assign(rec, await pageFacts(page));
          if (rec.ok && NOT_FOUND.test(rec.url)) {
            rec.ok = false;
            rec.error = 'HISv3 answered 404: route does not exist (or is not allowed) for this role';
          }
          if (step.action !== 'wait' || i === flow.steps.length - 1) {
            const file = path.join(dir, `step-${String(i + 1).padStart(2, '0')}.png`);
            await page.screenshot({ path: file, fullPage: false }).then(
              () => {
                rec.screenshot = path.relative(outDir, file);
                screenshots++;
              },
              () => undefined,
            );
          }
          steps.push(rec);
        }
        flowsOut.push({
          id: flow.id,
          name: flow.name,
          role,
          description: flow.description,
          ok: steps.every((s) => s.ok),
          steps,
        });
      }
    } catch (err) {
      if (signal.aborted) throw err;
      roleErrors[role] = errMsg(err);
    } finally {
      await context.close().catch(() => undefined);
    }
  }

  return {
    target: new URL(cfg.stagingUrl).origin,
    artifacts_dir: outDir,
    flows: flowsOut,
    role_errors: roleErrors,
    summary: {
      roles: params.roles.length,
      flows: flowsOut.length,
      failed_flows: flowsOut.filter((f) => !f.ok).length,
      screenshots,
    },
  };
}
