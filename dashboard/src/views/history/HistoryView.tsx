// Run history (docs/designs/RUN_HISTORY.md): builds, Playwright runs and knowledge syncs on real
// data. Successor of the removed /qa QA Insights page. Tab, filters and page live in the URL.
import { FormEvent, KeyboardEvent, useCallback, useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { RefreshCw } from 'lucide-react';
import { usePageHeader } from '../../components/pageHeader';
import { Product, api } from '../products/api';
import { inputCls, selectCls } from '../knowledge/styles';
import { DEFAULT_PAGE_SIZE, Filters } from './api';
import { FilterField } from './filters';
import BuildsTab from './BuildsTab';
import RunsTab from './RunsTab';
import SyncsTab from './SyncsTab';
import ConversationsTab from './ConversationsTab';

type TabId = 'builds' | 'runs' | 'syncs' | 'conversations';
const TABS: { id: TabId; label: string; keys: string[] }[] = [
  { id: 'builds', label: 'Builds', keys: ['type', 'status', 'case', 'epic', 'batch'] },
  { id: 'runs', label: 'Playwright runs', keys: ['source', 'outcome', 'case', 'epic', 'job'] },
  { id: 'syncs', label: 'Knowledge syncs', keys: ['status'] },
  { id: 'conversations', label: 'AI conversations', keys: ['kind', 'q'] },
];
const COMMON = ['product', 'from', 'to', 'page', 'page_size'];

export default function HistoryView() {
  const [params, setParams] = useSearchParams();
  const tab = (TABS.find((t) => t.id === params.get('tab'))?.id ?? 'builds') as TabId;
  const [refreshKey, setRefreshKey] = useState(0);
  const [products, setProducts] = useState<Product[]>([]);
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});

  useEffect(() => {
    const ac = new AbortController();
    api<{ products: Product[] }>('/api/products', { signal: ac.signal })
      .then((r) => setProducts(r.products ?? []))
      .catch(() => undefined); // the filter still works by id from the URL
    return () => ac.abort();
  }, []);

  const refresh = useCallback(() => setRefreshKey((n) => n + 1), []);
  usePageHeader({
    title: 'Run history',
    actions: (
      <button type="button" onClick={refresh} className="btn btn-secondary inline-flex items-center gap-1.5 h-8 px-3 text-sm">
        <RefreshCw className="w-4 h-4" aria-hidden="true" /> Refresh
      </button>
    ),
  });

  const allowed = [...COMMON, ...(TABS.find((t) => t.id === tab)?.keys ?? [])];
  const filters: Filters = {};
  for (const k of allowed) {
    const v = params.get(k);
    if (v) filters[k] = v;
  }
  if (!filters.page_size) filters.page_size = String(DEFAULT_PAGE_SIZE);

  const setFilter = useCallback(
    (key: string, value: string) => {
      setParams(
        (cur) => {
          const next = new URLSearchParams(cur);
          if (value) next.set(key, value);
          else next.delete(key);
          if (key !== 'page') next.delete('page');
          return next;
        },
        { replace: key !== 'page' },
      );
    },
    [setParams],
  );

  const setTab = (id: TabId) => {
    // Keep the shared filters (product, dates, page size); drop the other tab's own ones.
    const next = new URLSearchParams();
    next.set('tab', id);
    for (const k of ['product', 'from', 'to', 'page_size']) {
      const v = params.get(k);
      if (v) next.set(k, v);
    }
    setParams(next);
  };

  const onTabKey = (e: KeyboardEvent) => {
    const i = TABS.findIndex((t) => t.id === tab);
    const j = e.key === 'ArrowRight' ? (i + 1) % TABS.length : e.key === 'ArrowLeft' ? (i - 1 + TABS.length) % TABS.length : -1;
    if (j < 0) return;
    e.preventDefault();
    setTab(TABS[j].id);
    tabRefs.current[TABS[j].id]?.focus();
  };

  const badRange = !!filters.from && !!filters.to && filters.from > filters.to;
  const onSubmit = (e: FormEvent) => e.preventDefault();

  return (
    <div className="space-y-4 min-w-0">
      <p className="text-sm text-gray-600">
        How long automation builds took (machine time apart from time waiting for a human approval), every Playwright run the QA tools received, and knowledge sync durations.
      </p>

      <form onSubmit={onSubmit} className="card card-body flex flex-wrap items-end gap-3" aria-label="Filters">
        <FilterField label="Product">
          <select className={selectCls} value={filters.product ?? ''} onChange={(e) => setFilter('product', e.target.value)}>
            <option value="">All products</option>
            {products.map((p) => (
              <option key={p.id} value={String(p.id)}>
                {p.name}
              </option>
            ))}
            {filters.product && !products.some((p) => String(p.id) === filters.product) && <option value={filters.product}>Product {filters.product}</option>}
          </select>
        </FilterField>
        <FilterField label="From (UTC)">
          <input type="date" className={inputCls} value={filters.from ?? ''} onChange={(e) => setFilter('from', e.target.value)} />
        </FilterField>
        <FilterField label="To (UTC)">
          <input type="date" className={inputCls} value={filters.to ?? ''} onChange={(e) => setFilter('to', e.target.value)} />
        </FilterField>
        {badRange && (
          <p role="alert" className="text-sm text-red-700">
            "From" is after "To".
          </p>
        )}
      </form>

      <div role="tablist" aria-label="Run history sections" className="flex gap-1 border-b border-gray-200 overflow-x-auto" onKeyDown={onTabKey}>
        {TABS.map((t) => {
          const active = t.id === tab;
          return (
            <button
              key={t.id}
              ref={(el) => (tabRefs.current[t.id] = el)}
              type="button"
              role="tab"
              id={`history-tab-${t.id}`}
              aria-selected={active}
              aria-controls={`history-panel-${t.id}`}
              tabIndex={active ? 0 : -1}
              onClick={() => setTab(t.id)}
              className={`relative px-4 py-2 min-h-10 text-sm font-medium whitespace-nowrap ${active ? 'text-primary-700' : 'text-gray-600 hover:text-gray-900'}`}
            >
              {t.label}
              <span aria-hidden="true" className={`absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-primary-600 ${active ? 'opacity-100' : 'opacity-0'}`} />
            </button>
          );
        })}
      </div>

      <div role="tabpanel" id={`history-panel-${tab}`} aria-labelledby={`history-tab-${tab}`} className="min-w-0">
        {badRange ? null : tab === 'builds' ? (
          <BuildsTab filters={filters} setFilter={setFilter} refreshKey={refreshKey} />
        ) : tab === 'runs' ? (
          <RunsTab filters={filters} setFilter={setFilter} refreshKey={refreshKey} />
        ) : tab === 'syncs' ? (
          <SyncsTab filters={filters} setFilter={setFilter} refreshKey={refreshKey} />
        ) : (
          <ConversationsTab filters={filters} setFilter={setFilter} refreshKey={refreshKey} />
        )}
      </div>
    </div>
  );
}
