// Data helpers for the knowledge detail pages.
import { useEffect, useState } from 'react';
import { Annotation, Coverage, TestCase, TestCaseFilter, entityHref, errorText, getTree, isAbort, knowledgeHref, listAllTestCases } from '../api';
import type { Crumb } from './kit';

/** What every detail page needs besides its entity. */
export interface PageCtx {
  pid: number;
  productName: string | null;
  canWrite: boolean;
  onAnnotation: (a: Annotation | null) => void;
}

/** The first two crumbs every page starts with. */
export const baseCrumbs = (ctx: PageCtx): Crumb[] => [
  { label: ctx.productName ?? 'Product', to: `/products?product=${ctx.pid}` },
  { label: 'Knowledge base', to: knowledgeHref(ctx.pid) },
];

export interface TreeIndex {
  parts: Record<string, string>;
  modules: Record<string, { name: string; part: string }>;
  submodules: Record<string, { name: string; module: string }>;
}

// The tree is only used for breadcrumb names, so one copy per product per session is enough.
const treeCache = new Map<number, Promise<TreeIndex>>();

function loadIndex(pid: number): Promise<TreeIndex> {
  let p = treeCache.get(pid);
  if (!p) {
    p = getTree(pid, true).then((t) => {
      const idx: TreeIndex = { parts: {}, modules: {}, submodules: {} };
      for (const part of t.parts ?? []) {
        idx.parts[part.code] = part.name;
        for (const m of part.modules ?? []) {
          idx.modules[m.code] = { name: m.name, part: part.code };
          for (const s of m.submodules ?? []) idx.submodules[s.code] = { name: s.name, module: m.code };
        }
      }
      return idx;
    });
    p.catch(() => treeCache.delete(pid));
    treeCache.set(pid, p);
  }
  return p;
}

/** Names of parts, modules and sub-modules for breadcrumbs; null until loaded (or on failure). */
export function useTreeIndex(pid: number): TreeIndex | null {
  const [idx, setIdx] = useState<TreeIndex | null>(null);
  useEffect(() => {
    let live = true;
    loadIndex(pid)
      .then((i) => live && setIdx(i))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [pid]);
  return idx;
}

/** Every active test case in a scope (all pages). `scope` null means "nothing to load". */
export function useAllCases(pid: number, scope: TestCaseFilter | null) {
  const key = scope ? JSON.stringify(scope) : '';
  const [state, setState] = useState<{ key: string; cases: TestCase[] | null; error: string | null }>({ key: '', cases: null, error: null });
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    if (!scope) return;
    const ac = new AbortController();
    setState({ key, cases: null, error: null });
    listAllTestCases(pid, scope, ac.signal)
      .then((cases) => !ac.signal.aborted && setState({ key, cases, error: null }))
      .catch((e) => {
        if (ac.signal.aborted || isAbort(e)) return;
        setState({ key, cases: null, error: errorText(e, 'Could not load the test cases') });
      });
    return () => ac.abort();
    // `scope` is captured by `key`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pid, key, retry]);

  const current = state.key === key;
  return {
    cases: current ? state.cases : null,
    error: current ? state.error : null,
    loading: !!scope && (!current || (state.cases === null && state.error === null)),
    reload: () => setRetry((n) => n + 1),
  };
}

const NONE: TestCase[] = [];

/** Cases in a scope, skipping the request when the coverage already says there are none. */
export function useScopeCases(pid: number, scope: TestCaseFilter, coverage: Coverage | null | undefined) {
  const skip = !!coverage && coverage.test_cases === 0;
  const r = useAllCases(pid, skip ? null : scope);
  return skip ? { cases: NONE, error: null, loading: false, reload: r.reload } : r;
}

/** A breadcrumb to an entity page: the name when known, the code otherwise. */
export function crumbTo(pid: number, type: Parameters<typeof entityHref>[1], code: string | null | undefined, name?: string | null): Crumb[] {
  if (!code) return [];
  return [{ label: name || code, title: name ? `${code} ${name}` : code, to: entityHref(pid, type, code) }];
}
