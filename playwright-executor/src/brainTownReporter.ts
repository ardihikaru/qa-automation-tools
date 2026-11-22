/**
 * The Brain Town reporter (reporters/brain-town-reporter.cjs). case_run adds it to a Playwright run
 * only when BRAIN_TOWN_URL is set, the file loads, and EXECUTOR_BRAIN_TOWN_REPORTER is not 0.
 * Without a URL it would do nothing, so it is left out and the run is unchanged.
 */
import * as fs from 'fs';
import * as path from 'path';

/** dist/src/brainTownReporter.js -> <package>/reporters/brain-town-reporter.cjs. */
export const BRAIN_TOWN_REPORTER_PATH = path.resolve(__dirname, '..', '..', 'reporters', 'brain-town-reporter.cjs');

export interface BrainTownConfig {
  /** Absolute reporter path, or '' (off). */
  reporter: string;
  url: string;
  token: string;
  org: string;
  /** Default product key when the env file has no entry in `products`. */
  product: string;
  /** automation-<id>.env id -> Brain Town product key. */
  products: Record<string, string>;
}

type Env = Record<string, string | undefined>;

export function resolveBrainTownReporter(env: Env, file: string = BRAIN_TOWN_REPORTER_PATH): string {
  if (env.EXECUTOR_BRAIN_TOWN_REPORTER === '0') return '';
  if (!(env.BRAIN_TOWN_URL ?? '').trim()) return '';
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

/** BRAIN_TOWN_PRODUCTS="1=samplereport,2=sut" */
export function parseProductMap(raw: string | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const part of (raw ?? '').split(',')) {
    const [id, key] = part.split('=').map((s) => s.trim());
    if (id && key && /^[A-Za-z0-9_.-]{1,64}$/.test(key)) out[id] = key;
  }
  return out;
}

export function loadBrainTownConfig(env: Env, file: string = BRAIN_TOWN_REPORTER_PATH): BrainTownConfig {
  return {
    reporter: resolveBrainTownReporter(env, file),
    url: (env.BRAIN_TOWN_URL ?? '').trim(),
    token: (env.BRAIN_TOWN_TOKEN ?? '').trim(),
    org: (env.BRAIN_TOWN_ORG ?? 'example-org').trim(),
    product: (env.BRAIN_TOWN_PRODUCT ?? '').trim(),
    products: parseProductMap(env.BRAIN_TOWN_PRODUCTS),
  };
}

/** Product key for a run: the map entry for automation-<id>.env, else the default, else product-<id>. */
export function productFor(cfg: BrainTownConfig, envFile: string): string {
  const id = /^automation-(.+)\.env$/.exec(envFile)?.[1] ?? '';
  return cfg.products[id] || cfg.product || (id ? `product-${id}` : 'unknown');
}
