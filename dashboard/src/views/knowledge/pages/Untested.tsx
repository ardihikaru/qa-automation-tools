// One compact panel for an entity without test cases: what is missing, what to do about it,
// and where coverage does exist nearby (the parent's coverage, siblings with test cases).
import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { EntityDetail, EntityType, TYPE_LABEL, entityHref, knowledgeHref } from '../api';
import { ErrorNote, Skeleton } from '../ui';
import { CoverageCount, StatusBar } from './kit';
import { ContextItem, useCatalogUrl, useUntestedContext } from './context';
import type { TreeIndex } from './data';

const SINGULAR: Record<EntityType, string> = {
  part: 'part',
  module: 'module',
  submodule: 'sub-module',
  feature: 'feature',
  epic: 'epic',
  story: 'user story',
  test_case: 'test case',
};
const PLURAL: Record<EntityType, string> = {
  part: 'parts',
  module: 'modules',
  submodule: 'sub-modules',
  feature: 'features',
  epic: 'epics',
  story: 'user stories',
  test_case: 'test cases',
};

export interface UntestedProps {
  pid: number;
  e: EntityDetail;
  index: TreeIndex | null;
  /** What is missing, one sentence. */
  what: ReactNode;
  /** What to do. `catalog` is a link to docs/test-catalog, `sync` a link to the knowledge base. */
  todo: (catalog: ReactNode, sync: ReactNode) => ReactNode;
  /** Extra lines under the advice (e.g. the stories that link here). */
  extra?: ReactNode;
  /** Facts above the context (e.g. the release status). */
  facts?: ReactNode;
}

function ItemLine({ pid, item, strong }: { pid: number; item: ContextItem; strong?: boolean }) {
  return (
    <Link
      to={entityHref(pid, item.type, item.code)}
      title={`${item.code} ${item.name}`}
      className="kb-row grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 rounded-md px-2 py-1.5 min-h-8 focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary-600"
    >
      <span className="flex items-center gap-2 min-w-0">
        <span className="font-mono text-xs text-gray-500 shrink-0">{item.code}</span>
        <span className={`truncate text-sm ${strong ? 'font-medium text-gray-900' : 'text-gray-800'}`}>{item.name}</span>
      </span>
      <CoverageCount coverage={item.coverage} />
    </Link>
  );
}

function ContextSkeleton() {
  return (
    <div role="status" aria-label="Loading related coverage" className="space-y-3">
      <Skeleton className="h-3 w-40" />
      <Skeleton className="h-2 w-full max-w-xs" />
      <Skeleton className="h-3 w-56 mt-4" />
      {[0, 1, 2].map((i) => (
        <Skeleton key={i} className={`h-3 ${i % 2 ? 'w-2/3' : 'w-3/4'}`} />
      ))}
    </div>
  );
}

export default function UntestedPanel({ pid, e, index, what, todo, extra, facts }: UntestedProps) {
  const ctx = useUntestedContext(pid, e, index);
  const catalogUrl = useCatalogUrl(pid);
  const catalog = catalogUrl ? (
    <a href={catalogUrl} target="_blank" rel="noreferrer" className="font-mono text-[0.8125rem] text-primary-700 hover:underline">
      docs/test-catalog
    </a>
  ) : (
    <code className="font-mono text-[0.8125rem] text-gray-800">docs/test-catalog</code>
  );
  const sync = (
    <Link to={knowledgeHref(pid, 'syncs')} className="text-primary-700 hover:underline">
      sync the knowledge base
    </Link>
  );
  const c = ctx.data;
  const parent = c?.parent;
  const noun = PLURAL[e.type];
  // Where "the rest" lives: the parent's page, or the knowledge base for top-level types.
  const moreHref = parent ? entityHref(pid, parent.type, parent.code) : knowledgeHref(pid, e.type === 'epic' || e.type === 'story' ? 'stories' : 'modules');

  return (
    <section aria-labelledby="kb-untested" className="rounded-lg border border-gray-200 bg-white">
      <div className="grid 2xl:grid-cols-2 divide-y 2xl:divide-y-0 2xl:divide-x divide-gray-100">
        <div className="p-4 space-y-2 min-w-0">
          <h2 id="kb-untested" className="text-base font-semibold text-gray-900">
            No test cases yet
          </h2>
          <p className="text-sm text-gray-700">{what}</p>
          <p className="text-sm text-gray-700">{todo(catalog, sync)}</p>
          {extra}
        </div>
        <div className="p-4 space-y-4 min-w-0">
          {facts}
          {ctx.error && <ErrorNote onRetry={ctx.reload}>Could not load related coverage: {ctx.error}</ErrorNote>}
          {!c && !ctx.error && <ContextSkeleton />}
          {parent && (
            <div className="space-y-1.5">
              <p className="text-sm text-gray-500">
                In {TYPE_LABEL[parent.type].toLowerCase()} <span className="font-mono text-xs text-gray-700">{parent.code}</span>
              </p>
              <ItemLine pid={pid} item={parent} strong />
              <span className="block px-2">
                <StatusBar coverage={parent.coverage} className="w-full max-w-sm" />
              </span>
            </div>
          )}
          {c && (
            <div className="space-y-1.5">
              <p className="text-sm text-gray-500">
                {c.siblingsTotal > 0
                  ? `${c.siblingsTotal.toLocaleString()} other ${c.siblingsTotal === 1 ? SINGULAR[e.type] : noun}${parent ? ` in ${parent.code}` : ''} with test cases`
                  : `No other ${noun}${parent ? ` in ${parent.code}` : ''} have test cases yet.`}
              </p>
              {c.siblings.length > 0 && (
                <ul>
                  {c.siblings.map((s) => (
                    <li key={s.code}>
                      <ItemLine pid={pid} item={s} />
                    </li>
                  ))}
                </ul>
              )}
              {c.siblingsTotal > c.siblings.length && (
                <Link to={moreHref} className="inline-block px-2 text-sm text-primary-700 hover:underline">
                  {`${(c.siblingsTotal - c.siblings.length).toLocaleString()} more${parent ? ` in ${parent.code}` : ''}`}
                </Link>
              )}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
