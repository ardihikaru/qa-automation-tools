// The list behind /agents, /rules and the "Repo skills" view of /skills.
// URL state: `q` (search), `page`, and `kind` when `segmented` (rules page). The product
// filter belongs to the page (it is shared with other views on /skills) and comes in as props.
import { ReactNode, useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Search, X } from 'lucide-react';
import { useDebounced, useLoad } from '../knowledge/hooks';
import { inputCls } from '../knowledge/styles';
import { ErrorNote, Pagination } from '../knowledge/ui';
import { AssistantKind, KIND_META, listAssistants } from './api';
import { AssistantRow, EmptyState, RowsSkeleton, Segmented } from './ui';

const PAGE_SIZE = 50;

interface Props {
  /** Kinds this list may show. */
  kinds: AssistantKind[];
  /** Show a segmented filter over `kinds` ("All" plus one per kind) bound to `?kind=`. */
  segmented?: boolean;
  /** What to call the items, e.g. "agent" / "agents". */
  one: string;
  plural: string;
  productId: number;
  productName?: string;
  /** Extra controls placed before the search box (the product filter). */
  toolbar?: ReactNode;
}

export default function AssistantList({ kinds, segmented = false, one, plural, productId, productName, toolbar }: Props) {
  const [params, setParams] = useSearchParams();
  const q = (params.get('q') ?? '').trim();
  const page = Math.max(1, Number(params.get('page')) || 1);
  const kindParam = params.get('kind') ?? '';
  const kind = segmented && (kinds as string[]).includes(kindParam) ? (kindParam as AssistantKind) : '';

  const update = (patch: Record<string, string>, replace = false) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(patch)) {
      if (v) next.set(k, v);
      else next.delete(k);
    }
    setParams(next, { replace });
  };

  // Search box: typed text is debounced into `?q=`; a URL change (back/forward) flows back in.
  const [text, setText] = useState(q);
  const debounced = useDebounced(text, 300);
  useEffect(() => {
    if (debounced.trim() !== q) update({ q: debounced.trim(), page: '' }, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced]);
  useEffect(() => {
    setText((prev) => (prev.trim() === q ? prev : q));
  }, [q]);

  const kindArg = kind || kinds.join(',');
  const key = JSON.stringify([productId, kindArg, q, page]);
  const { data, loading, error, reload } = useLoad(key, (signal) =>
    listAssistants({ product_id: productId || undefined, kind: kindArg, q: q || undefined, page, page_size: PAGE_SIZE }, signal),
  );

  const counts = data?.kinds;
  const allCount = counts ? kinds.reduce((n, k) => n + (counts[k] ?? 0), 0) : undefined;
  const total = data?.total ?? 0;
  const items = data?.items ?? [];
  const where = (kind ? [kind] : kinds).map((k) => KIND_META[k].where);
  const shownPlural = kind ? KIND_META[kind].plural : plural;
  const shownOne = kind ? KIND_META[kind].one : one;

  // A page past the end (e.g. after the filter narrowed) goes back to the last page.
  useEffect(() => {
    if (data && page > 1 && data.items.length === 0 && data.total > 0) {
      update({ page: String(Math.max(1, Math.ceil(data.total / PAGE_SIZE))) }, true);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, page]);

  const whereList = (
    <>
      {where.map((w, i) => (
        <span key={w}>
          {i > 0 && (i === where.length - 1 ? ' or ' : ', ')}
          <code>{w}</code>
        </span>
      ))}
    </>
  );

  let body: ReactNode;
  if (error && !data) {
    body = (
      <div className="p-4">
        <ErrorNote onRetry={reload}>{error}</ErrorNote>
      </div>
    );
  } else if (!data) {
    body = <RowsSkeleton />;
  } else if (items.length === 0 && total === 0) {
    if (q) {
      body = (
        <EmptyState title={`No ${shownPlural} match "${q}"`}>
          <p>Search looks at the name, description and file path.</p>
          <button type="button" className="btn btn-secondary text-sm py-1.5" onClick={() => setText('')}>
            Clear search
          </button>
        </EmptyState>
      );
    } else if (productId) {
      body = (
        <EmptyState title={`${productName || 'This product'} has no ${shownPlural} yet`}>
          <p>Add files under {whereList} in its automation repo, then run Sync on the product&rsquo;s knowledge card.</p>
          <p>
            <Link to={`/products?product=${productId}`} className="text-primary-700 underline underline-offset-2">
              Open the product
            </Link>
          </p>
        </EmptyState>
      );
    } else {
      body = (
        <EmptyState title={`No ${shownPlural} synced yet`}>
          <p>They come from each product&rsquo;s automation repo. Add files under {whereList}, then run Sync on the product&rsquo;s knowledge card.</p>
          <p>
            <Link to="/products" className="text-primary-700 underline underline-offset-2">
              Go to products
            </Link>
          </p>
        </EmptyState>
      );
    }
  } else {
    body = (
      <ul className={`divide-y divide-gray-100 transition-opacity duration-150 ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
        {items.map((it) => (
          <AssistantRow key={`${it.product_id}:${it.code}`} it={it} showKind={segmented && !kind} />
        ))}
      </ul>
    );
  }

  return (
    <div className="space-y-4 min-w-0">
      {segmented && (
        <Segmented<AssistantKind | ''>
          label="File kind"
          value={kind}
          onChange={(v) => update({ kind: v, page: '' })}
          options={[
            { value: '', label: 'All', count: allCount },
            ...kinds.map((k) => ({ value: k, label: KIND_META[k].segment, count: counts?.[k] })),
          ]}
        />
      )}

      <div className="card min-w-0">
        <div className="flex flex-wrap items-center gap-3 px-4 py-3 border-b border-gray-200">
          {toolbar}
          <label className="relative flex-1 min-w-[12rem] max-w-md">
            <span className="sr-only">Search {shownPlural}</span>
            <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" aria-hidden="true" />
            <input
              type="search"
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Escape' && text) {
                  e.preventDefault();
                  setText('');
                }
              }}
              placeholder="Search name, description or path"
              className={`${inputCls} w-full pl-9 pr-8`}
              maxLength={200}
            />
            {text && (
              <button
                type="button"
                onClick={() => setText('')}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded p-1 text-gray-400 hover:text-gray-700"
                aria-label="Clear search"
              >
                <X className="w-3.5 h-3.5" aria-hidden="true" />
              </button>
            )}
          </label>
          <span className="ml-auto text-sm text-gray-600 tabular-nums" aria-live="polite">
            {data ? `${total} ${total === 1 ? shownOne : shownPlural}` : ''}
          </span>
        </div>

        {error && data && (
          <div className="px-4 pt-3">
            <ErrorNote onRetry={reload}>{error}</ErrorNote>
          </div>
        )}

        {body}

        {data && total > PAGE_SIZE && (
          <div className="border-t border-gray-200 px-4 py-3">
            <Pagination page={page} pageSize={PAGE_SIZE} total={total} disabled={loading} onPage={(p) => update({ page: p > 1 ? String(p) : '' })} />
          </div>
        )}
      </div>
    </div>
  );
}
