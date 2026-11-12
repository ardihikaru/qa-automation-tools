import { Browser, chromium } from 'playwright';

/**
 * Launches the bundled Chromium with low-memory flags. The systemd unit caps
 * the whole process tree (MemoryMax 2.5G, CPUQuota 60 % of CPUs); these flags
 * keep a single browser well inside that budget.
 *
 * --no-sandbox: the service runs unprivileged under a systemd user unit and
 * Ubuntu 24.04 restricts unprivileged user namespaces, so Chromium's own
 * sandbox cannot start. Only HISv3 staging is ever loaded.
 */
export async function launchBrowser(headless: boolean, signal: AbortSignal): Promise<Browser> {
  const browser = await chromium.launch({
    headless,
    args: [
      '--no-sandbox',
      '--disable-dev-shm-usage',
      '--disable-gpu',
      '--disable-extensions',
      '--renderer-process-limit=2',
      '--js-flags=--max-old-space-size=512',
    ],
  });
  const kill = () => void browser.close().catch(() => undefined);
  if (signal.aborted) kill();
  else signal.addEventListener('abort', kill, { once: true });
  return browser;
}

export function throwIfAborted(signal: AbortSignal): void {
  if (signal.aborted) {
    throw signal.reason instanceof Error ? signal.reason : new Error('run aborted');
  }
}
