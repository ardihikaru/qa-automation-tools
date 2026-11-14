// Context for pages of entities without test cases: the parent's coverage and the siblings in
// that parent that do have test cases, so an empty page still points somewhere useful.
import {
  Coverage,
  EMPTY_COVERAGE,
  EntityDetail,
  EntityType,
  Epic,
  FeatureDetail,
  ModuleDetail,
  ModuleItem,
  StoryDetail,
  Summary,
  SubmoduleDetail,
  getCoverage,
  getSummary,
  githubSlug,
  listEpics,
  listFeatures,
  listModules,
  listStories,
} from '../api';
import { CASE_STATUSES } from '../coverage';
import { useLoad } from '../hooks';
import type { TreeIndex } from './data';

export interface ContextItem {
  type: EntityType;
  code: string;
  name: string;
  coverage: Coverage;
}

export interface UntestedContext {
  parent: ContextItem | null;
  /** Siblings with test cases, most test cases first (at most SIBLINGS). */
  siblings: ContextItem[];
  /** Every sibling with test cases. */
  siblingsTotal: number;
}

const SIBLINGS = 6;
const TESTED = 'none,partial,full';

// Short-lived per-product caches: several pages in a row need the same module list and summary.
const TTL = 60_000;
const cache = new Map<string, { at: number; p: Promise<unknown> }>();
function cached<T>(key: string, load: () => Promise<T>): Promise<T> {
  const hit = cache.get(key);
  if (hit && Date.now() - hit.at < TTL) return hit.p as Promise<T>;
  const p = load();
  cache.set(key, { at: Date.now(), p });
  p.catch(() => cache.delete(key));
  return p;
}
const modulesOf = (pid: number) => cached<ModuleItem[]>(`${pid}:modules`, () => listModules(pid, false));
const summaryOf = (pid: number) => cached<Summary>(`${pid}:summary`, () => getSummary(pid));

/** A Coverage built from the epics list's per-status counts. */
function epicCoverage(e: Epic): Coverage {
  const c: Coverage = { ...EMPTY_COVERAGE };
  for (const s of CASE_STATUSES) (c[s.key] as number) = e.case_counts?.[s.status] ?? 0;
  c.test_cases = e.test_cases ?? 0;
  c.state = c.test_cases === 0 ? 'untested' : c.automated === 0 ? 'none' : c.automated < c.test_cases ? 'partial' : 'full';
  c.pct = c.test_cases ? (c.automated * 100) / c.test_cases : 0;
  return c;
}

const byCases = (a: ContextItem, b: ContextItem) => b.coverage.test_cases - a.coverage.test_cases || a.code.localeCompare(b.code, undefined, { numeric: true });

function fromMap(type: EntityType, cov: Record<string, Coverage>, name: (code: string) => string, skip: string): UntestedContext {
  const all = Object.entries(cov)
    .filter(([code, c]) => code !== skip && c.test_cases > 0)
    .map(([code, c]) => ({ type, code, name: name(code), coverage: c }))
    .sort(byCases);
  return { parent: null, siblings: all.slice(0, SIBLINGS), siblingsTotal: all.length };
}

async function load(pid: number, e: EntityDetail, index: TreeIndex | null, signal: AbortSignal): Promise<UntestedContext> {
  switch (e.type) {
    case 'feature': {
      const f = e as FeatureDetail;
      const [mods, sib] = await Promise.all([modulesOf(pid), listFeatures(pid, { module: f.module_code, coverage: TESTED, sort: 'tested', page_size: SIBLINGS + 1 }, signal)]);
      const m = mods.find((x) => x.code === f.module_code);
      const items = sib.items.filter((x) => x.code !== f.code).map((x) => ({ type: 'feature' as const, code: x.code, name: x.name, coverage: x.coverage }));
      return {
        parent: m ? { type: 'module', code: m.code, name: m.name, coverage: m.coverage } : null,
        siblings: items.sort(byCases).slice(0, SIBLINGS),
        // The page's own feature has no test cases, so it is never among these.
        siblingsTotal: sib.total,
      };
    }
    case 'submodule': {
      const s = e as SubmoduleDetail;
      const [mods, cov] = await Promise.all([modulesOf(pid), getCoverage(pid, 'submodule', { module: s.module_code }, signal)]);
      const m = mods.find((x) => x.code === s.module_code);
      return {
        ...fromMap('submodule', cov, (code) => index?.submodules[code]?.name ?? code, s.code),
        parent: m ? { type: 'module', code: m.code, name: m.name, coverage: m.coverage } : null,
      };
    }
    case 'module': {
      const m = e as ModuleDetail;
      const [mods, parts] = await Promise.all([modulesOf(pid), getCoverage(pid, 'part', {}, signal)]);
      const all = mods
        .filter((x) => x.part_code === m.part_code && x.code !== m.code && x.coverage.test_cases > 0)
        .map((x) => ({ type: 'module' as const, code: x.code, name: x.name, coverage: x.coverage }))
        .sort(byCases);
      const partName = m.part?.[0]?.name ?? index?.parts[m.part_code] ?? m.part_code;
      return {
        parent: { type: 'part', code: m.part_code, name: partName, coverage: parts[m.part_code] ?? EMPTY_COVERAGE },
        siblings: all.slice(0, SIBLINGS),
        siblingsTotal: all.length,
      };
    }
    case 'part': {
      const parts = await getCoverage(pid, 'part', {}, signal);
      return fromMap('part', parts, (code) => index?.parts[code] ?? code, e.code);
    }
    case 'epic': {
      const epics = await listEpics(pid, false, signal);
      const all = epics
        .filter((x) => x.code !== e.code && (x.test_cases ?? 0) > 0)
        .map((x) => ({ type: 'epic' as const, code: x.code, name: x.title, coverage: epicCoverage(x) }))
        .sort(byCases);
      return { parent: null, siblings: all.slice(0, SIBLINGS), siblingsTotal: all.length };
    }
    case 'story': {
      const s = e as StoryDetail;
      const [epics, sib] = await Promise.all([getCoverage(pid, 'epic', {}, signal), listStories(pid, { epic: s.epic_code, coverage: TESTED, page_size: SIBLINGS }, signal)]);
      return {
        parent: { type: 'epic', code: s.epic_code, name: s.epic?.[0]?.name ?? s.epic_code, coverage: epics[s.epic_code] ?? EMPTY_COVERAGE },
        siblings: sib.items.map((x) => ({ type: 'story' as const, code: x.code, name: x.text, coverage: x.coverage ?? EMPTY_COVERAGE })).sort(byCases),
        siblingsTotal: sib.total,
      };
    }
    default:
      return { parent: null, siblings: [], siblingsTotal: 0 };
  }
}

/** Loads the context once the page knows the entity has no test cases. */
export function useUntestedContext(pid: number, e: EntityDetail, index: TreeIndex | null) {
  // Names from the tree index only matter for parts and sub-modules.
  const needsIndex = e.type === 'part' || e.type === 'submodule';
  return useLoad<UntestedContext>(`${pid}:${e.type}:${e.code}:${needsIndex && index ? 'i' : ''}`, (s) => load(pid, e, index, s));
}

/** Link to the automation repo's test catalog folder (GitHub only), or null. */
export function useCatalogUrl(pid: number): string | null {
  const { data } = useLoad<Summary>(`${pid}:summary`, () => summaryOf(pid));
  const slug = githubSlug(data?.repo?.url);
  return slug ? `https://github.com/${slug}/tree/${encodeURI(data?.repo?.branch || 'main')}/docs/test-catalog` : null;
}
