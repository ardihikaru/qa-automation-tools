/**
 * The navigation reporter (reporters/nav-reporter.cjs, docs/designs/NAVIGATION_MAP.md 3.5). case_run
 * adds it to a Playwright run only when it is present, loads, and EXECUTOR_NAV_REPORTER is not 0:
 * a reporter Playwright cannot load would abort the whole run.
 */
import * as fs from 'fs';
import * as path from 'path';

/** dist/src/navReporter.js -> <package>/reporters/nav-reporter.cjs. */
export const NAV_REPORTER_PATH = path.resolve(__dirname, '..', '..', 'reporters', 'nav-reporter.cjs');

export function resolveNavReporter(env: Record<string, string | undefined>, file: string = NAV_REPORTER_PATH): string {
  if (env.EXECUTOR_NAV_REPORTER === '0') return '';
  // The path goes into a comma-separated --reporter list.
  if (file.includes(',')) return '';
  try {
    if (!fs.statSync(file).isFile()) return '';
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const mod = require(file);
    return typeof mod === 'function' ? file : '';
  } catch {
    return '';
  }
}
