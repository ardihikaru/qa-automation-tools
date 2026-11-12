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
import { loginAs } from '../sut/login';
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
      for (const flow of flowsForRole(role, params.flows)) {
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
