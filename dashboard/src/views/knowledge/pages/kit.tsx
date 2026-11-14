// Presentational pieces shared by the knowledge detail pages (and the list tabs).
import { ReactNode, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight, StickyNote } from 'lucide-react';
import { Coverage, EntityType, TestCase, entityHref } from '../api';
import { CASE_STATUSES, CHILD_FILTERS, ChildFilter, coverageSentence, excerpt, matchesFilter, statusLabel } from '../coverage';
import { HideUntestedToggle, LocalBadge, UntestedRunButton } from '../ui';
import { UNTESTED_RUN, isTested, testedFirst } from '../paging';
import { InlineCode } from '../EntityPanel';

// ---------------------------------------------------------------- status vocabulary

/** A small status swatch (same fill as the grid square). */
export function StatusSwatch({ status, size = 10 }: { status: string; size?: number }) {
  return <span aria-hidden="true" data-s={status} className="kb-st inline-block shrink-0 rounded-[2px]" style={{ width: size, height: size }} />;
}

export function StatusLabel({ status, automated }: { status: string; automated?: boolean }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-gray-700 whitespace-nowrap">
      <StatusSwatch status={status} />
      {statusLabel(status)}
      {automated && status !== 'automated' && <span className="text-gray-400">(spec found)</span>}
    </span>
  );
}

/** Thin proportional bar: automated, planned, blocked, manual only, won't. Hatched outline when untested. */
export function StatusBar({ coverage, className = 'w-40' }: { coverage: Coverage | null | undefined; className?: string }) {
  const c = coverage;
  const label = coverageSentence(c);
  if (!c || c.test_cases === 0) return <span role="img" aria-label={label} title={label} className={`kb-bar-empty block ${className}`} />;
  return (
    <span role="img" aria-label={label} title={label} className={`kb-bar ${className}`}>
      {CASE_STATUSES.map((s) => {
        const n = c[s.key] as number;
        return n > 0 ? <span key={s.status} data-s={s.status} className="kb-st" style={{ flexGrow: n, flexBasis: 0 }} /> : null;
      })}
    </span>
  );
}

/** "12 of 40" with tabular numerals, or a quiet "No test cases yet". */
export function CoverageCount({ coverage }: { coverage: Coverage | null | undefined }) {
  if (!coverage || coverage.test_cases === 0) return <span className="text-xs text-gray-400 whitespace-nowrap">No test cases yet</span>;
  return (
    <span className="text-xs text-gray-600 tabular-nums whitespace-nowrap" title={coverageSentence(coverage)}>
      {coverage.automated.toLocaleString()} of {coverage.test_cases.toLocaleString()} automated
    </span>
  );
}

// ---------------------------------------------------------------- breadcrumbs

export interface Crumb {
  label: ReactNode;
  to?: string;
  title?: string;
}

export function Crumbs({ items }: { items: Crumb[] }) {
  return (
    <nav aria-label="Breadcrumb">
      <ol className="flex flex-wrap items-center gap-x-1 gap-y-1 text-sm text-gray-500 min-w-0">
        {items.map((c, i) => {
          const last = i === items.length - 1;
          return (
            <li key={i} className="inline-flex items-center gap-1 min-w-0 max-w-full">
              {c.to && !last ? (
                <Link to={c.to} title={c.title} className="truncate max-w-[16rem] text-primary-600 hover:underline min-h-8 inline-flex items-center">
                  <span className="truncate">{c.label}</span>
                </Link>
              ) : (
                <span aria-current={last ? 'page' : undefined} title={c.title} className={`truncate max-w-[16rem] ${last ? 'text-gray-700' : ''}`}>
                  {c.label}
                </span>
              )}
              {!last && <ChevronRight className="w-3.5 h-3.5 shrink-0 text-gray-400" aria-hidden="true" />}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

// ---------------------------------------------------------------- sections

export function SectionHead({ id, title, count, children }: { id?: string; title: ReactNode; count?: number; children?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
      <h2 id={id} className="text-base font-semibold text-gray-900">
        {title}
        {count !== undefined && <span className="ml-2 text-sm font-normal text-gray-500 tabular-nums">{count.toLocaleString()}</span>}
      </h2>
      {children}
    </div>
  );
}

export function Fields({ rows }: { rows: [string, ReactNode][] }) {
  const shown = rows.filter(([, v]) => v !== '' && v !== null && v !== undefined && v !== false);
  if (shown.length === 0) return null;
  return (
    <dl className="grid grid-cols-[7rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
      {shown.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-gray-500">{k}</dt>
          <dd className="text-gray-900 break-words min-w-0">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

export function FilterChips({ value, onChange, counts, label }: { value: ChildFilter; onChange: (f: ChildFilter) => void; counts: Record<ChildFilter, number>; label: string }) {
  return (
    <div role="group" aria-label={label} className="inline-flex flex-wrap gap-1 rounded-lg border border-gray-200 bg-white p-0.5">
      {CHILD_FILTERS.map((f) => {
        const on = f.id === value;
        return (
          <button
            key={f.id}
            type="button"
            aria-pressed={on}
            title={f.title}
            onClick={() => onChange(f.id)}
            className={`inline-flex items-center gap-1.5 rounded-md px-2.5 min-h-8 text-sm ${on ? 'bg-primary-50 text-primary-800 font-medium' : 'text-gray-600 hover:text-gray-900'}`}
          >
            {f.label}
            <span className={`tabular-nums text-xs ${on ? 'text-primary-700' : 'text-gray-400'}`}>{counts[f.id].toLocaleString()}</span>
          </button>
        );
      })}
    </div>
  );
}

/**
 * A list section whose rows can be filtered by coverage state. Tested rows come first; a long
 * run of untested rows folds into one "+ N … without test cases" line, and "Hide untested"
 * drops them. A section with nothing tested shows just that folded line.
 */
export function CoverageSection<T extends { code: string; coverage?: Coverage }>({
  id,
  title,
  items,
  empty,
  noun,
  render,
}: {
  id: string;
  title: string;
  items: T[];
  empty: ReactNode;
  /** Plural noun for the folded line ("features"). */
  noun: string;
  render: (item: T) => ReactNode;
}) {
  const [filter, setFilter] = useState<ChildFilter>('all');
  const [hide, setHide] = useState(false);
  const [unfolded, setUnfolded] = useState(false);
  const ordered = testedFirst(items, (i) => isTested(i.coverage));
  const untested = items.filter((i) => !isTested(i.coverage)).length;
  const mixed = untested > 0 && untested < items.length;
  const counts = Object.fromEntries(CHILD_FILTERS.map((f) => [f.id, items.filter((i) => matchesFilter(i.coverage, f.id)).length])) as Record<ChildFilter, number>;
  const matching = ordered.filter((i) => matchesFilter(i.coverage, filter) && !(hide && mixed && !isTested(i.coverage)));
  const runLen = matching.filter((i) => !isTested(i.coverage)).length;
  // Only fold when the user has not asked for untested rows explicitly.
  const folded = filter !== 'untested' && !unfolded && runLen > UNTESTED_RUN;
  const shown = folded ? matching.filter((i) => isTested(i.coverage)) : matching;
  return (
    <section aria-labelledby={id} className="space-y-3">
      <SectionHead id={id} title={title} count={items.length}>
        {mixed && (
          <span className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <HideUntestedToggle checked={hide} onChange={setHide} hidden={untested} />
            <FilterChips value={filter} onChange={setFilter} counts={counts} label={`Filter ${title.toLowerCase()}`} />
          </span>
        )}
      </SectionHead>
      {items.length === 0 ? (
        <p className="text-sm text-gray-500">{empty}</p>
      ) : matching.length === 0 ? (
        <p className="text-sm text-gray-500 rounded-lg border border-dashed border-gray-300 px-3 py-4">Nothing in this view.</p>
      ) : (
        <ul className="rounded-lg border border-gray-200 bg-white divide-y divide-gray-100 overflow-hidden">
          {shown.map((i) => (
            <li key={i.code}>{render(i)}</li>
          ))}
          {folded && (
            <li>
              <UntestedRunButton count={runLen} noun={noun} onClick={() => setUnfolded(true)} />
            </li>
          )}
        </ul>
      )}
    </section>
  );
}

/** A child row: code and name link to the page, then an optional extra, the bar and the count. */
export function CoverageRow({
  pid,
  type,
  code,
  name,
  origin,
  removed,
  coverage,
  extra,
  noted,
  wrap,
}: {
  pid: number;
  type: EntityType;
  code: string;
  name: string;
  origin: string;
  removed: boolean;
  coverage: Coverage | null | undefined;
  extra?: ReactNode;
  noted?: boolean;
  /** Let a long name wrap to two lines (stories) instead of truncating. */
  wrap?: boolean;
}) {
  return (
    <Link
      to={entityHref(pid, type, code)}
      className="kb-row grid grid-cols-[minmax(0,1fr)_auto] md:grid-cols-[minmax(0,1fr)_auto_10rem_8rem] items-center gap-x-4 gap-y-1.5 px-3 py-2.5 min-h-10 focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-primary-600"
    >
      <span className="flex items-center gap-2 min-w-0">
        <span className={`font-mono text-xs shrink-0 ${removed ? 'line-through text-gray-400' : 'text-gray-500'}`}>{code}</span>
        <span className={`${wrap ? 'line-clamp-2 break-words' : 'truncate'} text-sm ${removed ? 'line-through text-gray-400' : 'text-gray-900'}`} title={name}>
          {name || <span className="text-gray-400">Untitled</span>}
        </span>
        {origin === 'local' && <LocalBadge />}
        {noted && <StickyNote className="w-3.5 h-3.5 text-amber-500 shrink-0" aria-label="has a note" />}
      </span>
      <span className="justify-self-end md:justify-self-start min-w-0">{extra}</span>
      <StatusBar coverage={coverage} className="w-full" />
      <span className="justify-self-end">
        <CoverageCount coverage={coverage} />
      </span>
    </Link>
  );
}

// ---------------------------------------------------------------- test case rows

function SpecList({ paths }: { paths: string[] | null }) {
  if (!paths?.length) return null;
  return (
    <ul className="space-y-0.5">
      {paths.map((p) => (
        <li key={p} className="font-mono text-xs text-gray-500 break-all">
          {p}
        </li>
      ))}
    </ul>
  );
}

/** Test cases as rows: code, scenario excerpt, level, priority, status, spec paths. Stacks on narrow screens. */
export function CaseRows({ pid, cases }: { pid: number; cases: TestCase[] }) {
  return (
    <ul className="divide-y divide-gray-100">
      {cases.map((c) => {
        const removed = !!c.removed_at;
        return (
          <li key={c.code} className="grid grid-cols-[minmax(0,1fr)_auto] md:grid-cols-[11rem_minmax(0,1fr)_3.5rem_3.5rem_9rem] items-start gap-x-4 gap-y-1 px-3 py-2">
            <span className="flex items-center gap-1.5 min-w-0 md:min-h-6">
              <Link to={entityHref(pid, 'test_case', c.code)} className={`font-mono text-xs hover:underline truncate ${removed ? 'line-through text-gray-400' : 'text-primary-700'}`}>
                {c.code}
              </Link>
              {c.origin === 'local' && <LocalBadge />}
            </span>
            <span className="md:order-last justify-self-end md:justify-self-start md:min-h-6 flex items-center">
              <StatusLabel status={c.status} automated={c.automated} />
            </span>
            <span className="col-span-2 md:col-span-1 md:order-none min-w-0 space-y-1">
              <span className={`block text-sm break-words ${removed ? 'line-through text-gray-400' : 'text-gray-800'}`}><InlineCode text={excerpt(c.scenario, 220)} />
              </span>
              <SpecList paths={c.spec_paths} />
            </span>
            <span className="text-xs text-gray-600 md:min-h-6 flex items-center" title="Level">
              <span className="md:hidden text-gray-400 mr-1">Level</span>
              {c.level}
            </span>
            <span className="text-xs text-gray-600 md:min-h-6 flex items-center justify-self-end md:justify-self-start" title="Priority">
              <span className="md:hidden text-gray-400 mr-1">Priority</span>
              {c.priority}
            </span>
          </li>
        );
      })}
    </ul>
  );
}

/** "Linked features" line under a story or test case header. */
export function LinkedFeatures({ pid, refs, unmapped }: { pid: number; refs: { code: string; name: string; removed_at: string | null }[]; unmapped?: string | null }) {
  return (
    <p className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm">
      <span className="text-gray-500">Linked features</span>
      {refs.length === 0 ? (
        <span className="text-gray-500">{unmapped ? `None (unmapped: ${unmapped})` : 'None'}</span>
      ) : (
        refs.map((f) => (
          <Link key={f.code} to={entityHref(pid, 'feature', f.code)} title={f.name} className={`hover:underline min-w-0 max-w-full truncate ${f.removed_at ? 'line-through text-gray-400' : 'text-primary-700'}`}>
            <span className="font-mono text-xs">{f.code}</span> {f.name}
          </Link>
        ))
      )}
    </p>
  );
}
