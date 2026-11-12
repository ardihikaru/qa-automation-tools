import { Browser, BrowserContext } from 'playwright';
import { ExecutorConfig } from '../config';

export interface RunContext {
  cfg: ExecutorConfig;
  browser: Browser;
  signal: AbortSignal;
  runId: string;
  /** Absolute directory for this run's artifacts. */
  outDir: string;
}

export function newRoleContext(browser: Browser): Promise<BrowserContext> {
  return browser.newContext({
    viewport: { width: 1280, height: 720 },
    ignoreHTTPSErrors: false,
    serviceWorkers: 'block',
  });
}

export const errMsg = (err: unknown) => (err instanceof Error ? err.message.split('\n')[0] : String(err));
