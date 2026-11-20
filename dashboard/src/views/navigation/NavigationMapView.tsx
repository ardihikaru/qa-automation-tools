// /navigation: the product's pages and API routes, coloured by test coverage, with a drill-down and
// the next cases to automate (docs/designs/NAVIGATION_MAP.md section 7). It replaces the crawler's
// "Navigation Graph", which had no data source any more.
import { useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { ChevronDown, ChevronRight, Eye, Search, Sparkles } from 'lucide-react';
import { usePageHeader } from '../../components/pageHeader';
import { entityHref, knowledgeHref, shortSha } from '../knowledge/api';
import { useDebounced, useLoad } from '../knowledge/hooks';
import { Empty, ErrorNote, RelTime, SkeletonRows } from '../knowledge/ui';
import { inputCls, selectCls } from '../knowledge/styles';
import {
  NavFilter,
  NavMap,
  NextItem,
  ProductLite,
  RouteStatus,
  STATUSES,
  STATUS_DOT,
  STATUS_LABEL,
  defaultProduct,
  getNavigation,
  listProductsLite,
} from './api';
import { AppTree, TreeNode, allIds, buildTrees, compact } from './tree';
import RoutePanel from './RoutePanel';

function StatusDot({ status }: { status: RouteStatus }) {
  return <span className={`inline-block w-2.5 h-2.5 rounded-full shrink-0 ${STATUS_DOT[status]}`} title={STATUS_LABEL[status]} aria-label={STATUS_LABEL[status]} />;
}

/** A segment's subtree as a thin stacked bar of route statuses. */
function StatusBar({ counts, total }: { counts: Record<RouteStatus, number>; total: number }) {
  if (total === 0) return null;
  const label = STATUSES.filter((s) => counts[s] > 0)
    .map((s) => `${counts[s]} ${STATUS_LABEL[s].toLowerCase()}`)
    .join(', ');
  return (
    <span className="flex h-1.5 w-16 rounded-full overflow-hidden bg-gray-100 shrink-0" title={label} aria-label={label}>
      {STATUSES.map((s) => (counts[s] > 0 ? <span key={s} className={STATUS_DOT[s]} style={{ width: `${(counts[s] / total) * 100}%` }} /> : null))}
    </span>
  );
}

interface RowProps {
  n: TreeNode;
  depth: number;
  open: Set<string>;
  toggle: (id: string) => void;
  selected: string;
  onSelect: (key: string) => void;
}

function TreeRow({ n, depth, open, toggle, selected, onSelect }: RowProps) {
  const hasKids = n.children.length > 0;
  const isOpen = open.has(n.id);
  const r = n.route;
  const active = !!r && r.key === selected;
  return (
    <li>
      <div
        className={`flex items-center gap-2 pr-2 py-1.5 border-l-2 ${active ? 'bg-primary-50 border-primary-500' : 'border-transparent hover:bg-gray-50'}`}
        style={{ paddingLeft: `${0.25 + depth * 1}rem` }}
      >
        {hasKids ? (
          <button type="button" onClick={() => toggle(n.id)} className="p-0.5 rounded text-gray-400 hover:text-gray-700 shrink-0" aria-expanded={isOpen} aria-label={`${isOpen ? 'Collapse' : 'Expand'} ${n.path}`}>
            {isOpen ? <ChevronDown className="w-4 h-4" aria-hidden="true" /> : <ChevronRight className="w-4 h-4" aria-hidden="true" />}
          </button>
        ) : (
          <span className="w-5 shrink-0" aria-hidden="true" />
        )}
        {r ? (
          <button type="button" onClick={() => onSelect(r.key)} className="flex items-center gap-2 min-w-0 flex-1 text-left" aria-current={active ? 'true' : undefined}>
            <StatusDot status={r.status} />
            <span className="font-mono text-sm text-gray-900 truncate" title={r.path}>
              {n.segment === '/' ? '/' : n.segment}
            </span>
            {r.methods.length > 0 && <span className="hidden sm:inline font-mono text-[11px] text-gray-400 truncate">{r.methods.join(' ')}</span>}
          </button>
        ) : (
          <button type="button" onClick={() => toggle(n.id)} className="flex items-center gap-2 min-w-0 flex-1 text-left">
            <span className="font-mono text-sm text-gray-500 truncate" title={n.path}>
              {n.segment}
            </span>
          </button>
        )}
        {r && r.observed_hits > 0 && <Eye className="w-3.5 h-3.5 text-violet-500 shrink-0" aria-label="Seen in a run" />}
        {hasKids && (
          <span className="hidden sm:flex">
            <StatusBar counts={n.counts} total={n.routes} />
          </span>
        )}
        {r ? (
          <span className="text-xs tabular-nums text-gray-500 shrink-0 w-20 sm:w-24 text-right" title={`${r.automated} automated of ${r.cases} cases; ${r.pass} pass, ${r.fail} fail`}>
            {r.cases > 0 ? `${r.automated}/${r.cases}` : '—'}
            {r.fail > 0 && <span className="ml-1.5 text-red-600">{r.fail}✕</span>}
          </span>
        ) : (
          <span className="text-xs tabular-nums text-gray-400 shrink-0 w-20 sm:w-24 text-right">{n.routes} routes</span>
        )}
      </div>
      {hasKids && isOpen && (
        <ul>
          {n.children.map((c) => (
            <TreeRow key={c.id} n={c} depth={depth + 1} open={open} toggle={toggle} selected={selected} onSelect={onSelect} />
          ))}
        </ul>
      )}
    </li>
  );
}

function AppSection({ t, open, toggle, selected, onSelect }: { t: AppTree } & Omit<RowProps, 'n' | 'depth'>) {
  const root = compact(t.root);
  const rows = root.route ? [root] : root.children;
  return (
    <section className="card">
      <div className="card-header flex flex-wrap items-center gap-2">
        <span className={`badge ${t.surface === 'web' ? 'bg-indigo-100 text-indigo-800' : 'bg-teal-100 text-teal-800'}`}>{t.surface === 'web' ? 'Pages' : 'API'}</span>
        <h2 className="font-semibold text-gray-900">{t.app}</h2>
        <span className="ml-auto flex items-center gap-2 text-xs text-gray-500">
          <StatusBar counts={t.root.counts} total={t.root.routes} />
          <span className="tabular-nums">{t.root.routes} routes</span>
        </span>
      </div>
      <ul className="py-1">
        {rows.map((n) => (
          <TreeRow key={n.id} n={n} depth={0} open={open} toggle={toggle} selected={selected} onSelect={onSelect} />
        ))}
      </ul>
    </section>
  );
}

function NextList({ pid, items, onSelect }: { pid: number; items: NextItem[]; onSelect: (key: string) => void }) {
  return (
    <section className="card">
      <div className="card-header flex items-center gap-2">
        <Sparkles className="w-4 h-4 text-amber-500" aria-hidden="true" />
        <h2 className="font-semibold text-gray-900">Next to automate</h2>
        <span className="text-xs text-gray-500">planned cases on routes that already have a passing spec</span>
      </div>
      <div className="card-body">
        {items.length === 0 ? (
          <Empty>No planned case shares a route with a passing automated case.</Empty>
        ) : (
          <ol className="divide-y divide-gray-100">
            {items.map((it) => (
              <li key={it.case_code} className="py-2 flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-3">
                <span className="flex items-center gap-2 shrink-0">
                  <span className={`badge ${it.tier === 1 ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-700'}`} title={it.tier === 1 ? 'Same route and method as a passing case' : 'Same route, another method'}>
                    {it.tier === 1 ? 'same call' : 'same route'}
                  </span>
                  <Link to={entityHref(pid, 'test_case', it.case_code)} className="font-mono text-xs text-primary-700 hover:underline">
                    {it.case_code}
                  </Link>
                  {it.priority && <span className="text-xs text-gray-500">{it.priority}</span>}
                </span>
                <button type="button" onClick={() => onSelect(it.route_key)} className="font-mono text-xs text-gray-700 hover:text-primary-700 text-left break-all min-w-0 flex-1">
                  {it.method && <span className="text-gray-400 mr-1">{it.method}</span>}
                  {it.path}
                </button>
                <span className="text-xs text-gray-500 shrink-0">
                  like{' '}
                  <Link to={entityHref(pid, 'test_case', it.example_case)} className="font-mono text-primary-700 hover:underline">
                    {it.example_case}
                  </Link>
                </span>
              </li>
            ))}
          </ol>
        )}
      </div>
    </section>
  );
}

export default function NavigationMapView() {
  usePageHeader({ title: 'Navigation map', titleHint: 'Pages and API routes of the product, coloured by test coverage' });
  const [params, setParams] = useSearchParams();
  const products = useLoad<ProductLite[]>('products', (s) => listProductsLite(s));
  const pidParam = Number(params.get('product') || '0');
  const pid = pidParam > 0 ? pidParam : defaultProduct(products.data ?? [])?.id ?? 0;

  const surface = (params.get('surface') ?? '') as NavFilter['surface'];
  const app = params.get('app') ?? '';
  const status = (params.get('status') ?? '') as NavFilter['status'];
  const selected = params.get('route') ?? '';
  const [query, setQuery] = useState(params.get('q') ?? '');
  const q = useDebounced(query.trim(), 300);

  const set = (k: string, v: string) =>
    setParams(
      (p) => {
        const n = new URLSearchParams(p);
        if (v) n.set(k, v);
        else n.delete(k);
        return n;
      },
      { replace: true },
    );
  useEffect(() => {
    if ((params.get('q') ?? '') !== q) set('q', q);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);

  const filter: NavFilter = { surface, app, status, q };
  const nav = useLoad<NavMap | null>(`${pid}:${JSON.stringify(filter)}`, (s) => (pid > 0 ? getNavigation(pid, filter, s) : Promise.resolve(null)));
  const data = nav.data && nav.data.product_id === pid ? nav.data : null;

  const trees = useMemo(() => buildTrees(data?.routes ?? []), [data]);
  const [open, setOpen] = useState<Set<string>>(new Set());
  // Open the first two levels by default; everything when searching or filtering by status.
  useEffect(() => {
    const ids = new Set<string>();
    for (const t of trees) {
      const root = compact(t.root);
      if (q || status) allIds(root).forEach((id) => ids.add(id));
      else for (const c of root.route ? [root] : root.children) ids.add(c.id);
    }
    setOpen(ids);
  }, [trees, q, status]);
  const toggle = (id: string) =>
    setOpen((cur) => {
      const n = new Set(cur);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

  const apps = (data?.apps ?? []).filter((a) => !surface || a.surface === surface);
  const totals = data?.totals;
  const build = data?.build ?? null;

  return (
    <div className="space-y-4">
      {/* Product and filters */}
      <div className="flex flex-wrap items-center gap-2">
        <select
          value={pid || ''}
          onChange={(e) => setParams(new URLSearchParams(e.target.value ? { product: e.target.value } : {}), { replace: true })}
          aria-label="Product"
          className={`${selectCls} max-w-[14rem]`}
        >
          {(products.data ?? []).map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <div className="inline-flex rounded-lg border border-gray-300 overflow-hidden text-sm" role="group" aria-label="Surface">
          {(['', 'web', 'api'] as const).map((s) => (
            <button
              key={s || 'all'}
              type="button"
              onClick={() => {
                set('surface', s);
                if (app) set('app', '');
              }}
              aria-pressed={surface === s}
              className={`h-9 px-3 ${surface === s ? 'bg-primary-600 text-white' : 'bg-white text-gray-700 hover:bg-gray-50'} ${s ? 'border-l border-gray-300' : ''}`}
            >
              {s === '' ? 'All' : s === 'web' ? 'Pages' : 'API'}
            </button>
          ))}
        </div>
        <select value={app} onChange={(e) => set('app', e.target.value)} aria-label="App or service" className={`${selectCls} max-w-[16rem]`}>
          <option value="">All apps and services</option>
          {apps.map((a) => (
            <option key={`${a.surface}:${a.app}`} value={a.app}>
              {a.app} ({a.routes})
            </option>
          ))}
        </select>
        <label className="relative flex-1 min-w-[12rem]">
          <Search className="w-4 h-4 text-gray-400 absolute left-2.5 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Path, page object or case code" aria-label="Search routes" className={`${inputCls} w-full pl-8`} />
        </label>
      </div>

      {/* Status counters (each one a filter) */}
      {totals && (
        <div className="flex flex-wrap items-center gap-2">
          {STATUSES.map((s: RouteStatus) => (
            <button
              key={s}
              type="button"
              onClick={() => set('status', status === s ? '' : s)}
              aria-pressed={status === s}
              className={`inline-flex items-center gap-2 h-8 px-3 rounded-full border text-sm ${status === s ? 'border-primary-500 bg-primary-50 text-primary-800' : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50'}`}
            >
              <StatusDot status={s} />
              <span>{STATUS_LABEL[s]}</span>
              <span className="tabular-nums font-semibold">{totals[s]}</span>
            </button>
          ))}
          <span className="inline-flex items-center gap-1.5 h-8 px-3 text-sm text-gray-600" title="Routes a real executor run visited">
            <Eye className="w-3.5 h-3.5 text-violet-500" aria-hidden="true" />
            seen in runs <span className="tabular-nums font-semibold">{totals.observed}</span>
          </span>
          <span className="text-xs text-gray-500 sm:ml-auto" title="Cases with no REST route or page: GraphQL operations, message-queue consumers and data-driven chains are not mapped yet">
            {totals.unmapped_cases} cases without a route (GraphQL, MQ, data-driven)
          </span>
        </div>
      )}

      {build && (
        <p className="text-xs text-gray-500">
          Built from {build.sync_id ? `knowledge sync #${build.sync_id}` : 'the last sync'}
          {build.commit && (
            <>
              {' '}
              at <code>{shortSha(build.commit)}</code>
            </>
          )}
          , <RelTime at={build.built_at} />
          {build.status === 'failed' && <span className="text-red-600"> · the last build failed: {build.error}</span>}
          {build.problems.length > 0 && <span title={build.problems.join('\n')}> · {build.problems.length} problems</span>}
        </p>
      )}

      {products.error && <ErrorNote onRetry={products.reload}>{products.error}</ErrorNote>}
      {nav.error && <ErrorNote onRetry={nav.reload}>{nav.error}</ErrorNote>}
      {products.data && products.data.length === 0 && <Empty>No product yet. Add one on the Products page.</Empty>}
      {(nav.loading || products.loading) && !data && <SkeletonRows rows={8} label="Loading the navigation map" />}

      {data && !build && (
        <Empty>
          No navigation map for this product yet. It is built by the knowledge sync:{' '}
          <Link to={knowledgeHref(pid)} className="text-primary-700 hover:underline">
            run a knowledge sync
          </Link>
          .
        </Empty>
      )}

      {data && build && (
        <div className={`grid gap-4 ${selected ? 'lg:grid-cols-[minmax(0,1fr)_minmax(20rem,26rem)]' : ''}`}>
          <div className="space-y-4 min-w-0">
            {trees.length === 0 ? (
              <Empty>No route matches these filters.</Empty>
            ) : (
              trees.map((t) => <AppSection key={`${t.surface}:${t.app}`} t={t} open={open} toggle={toggle} selected={selected} onSelect={(k) => set('route', k)} />)
            )}
            <NextList pid={pid} items={data.next} onSelect={(k) => set('route', k)} />
          </div>
          {selected && (
            <div className="min-w-0 order-first lg:order-none lg:sticky lg:top-4 lg:self-start lg:max-h-[calc(100vh-6rem)] lg:overflow-y-auto">
              <RoutePanel pid={pid} routeKey={selected} onClose={() => set('route', '')} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
