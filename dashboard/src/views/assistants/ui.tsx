// Presentational pieces shared by /agents, /rules, the repo skills list on /skills and the
// assistant detail page.
import { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { selectCls } from '../knowledge/styles';
import { AssistantItem, AssistantKind, KIND_META, ProductOption, detailPath, fmList, fmString } from './api';

/** Product filter. `extra` adds options before the products (e.g. "Global only" for QA checks). */
export function ProductSelect({
  value,
  onChange,
  products,
  failed,
  extra,
  id = 'product-filter',
}: {
  value: string;
  onChange: (v: string) => void;
  products: ProductOption[];
  failed?: boolean;
  extra?: { value: string; label: string }[];
  id?: string;
}) {
  // Keep an unknown id selectable so a shared link still shows what it filters on.
  const known = value === '' || extra?.some((o) => o.value === value) || products.some((p) => String(p.id) === value);
  return (
    <label htmlFor={id} className="inline-flex items-center gap-2 min-w-0">
      <span className="text-sm text-gray-600 shrink-0">Product</span>
      <select id={id} value={value} onChange={(e) => onChange(e.target.value)} className={`${selectCls} max-w-[16rem] truncate`}>
        <option value="">All products</option>
        {extra?.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
        {products.map((p) => (
          <option key={p.id} value={String(p.id)}>
            {p.name}
          </option>
        ))}
        {!known && <option value={value}>Product #{value}</option>}
      </select>
      {failed && <span className="text-xs text-amber-700">Could not load products</span>}
    </label>
  );
}

/** A two- or more-way segmented control. Pure colour change on select: it is a frequent action. */
export function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: ReactNode; count?: number }[];
  onChange: (v: T) => void;
}) {
  return (
    <div role="group" aria-label={label} className="inline-flex max-w-full overflow-x-auto rounded-lg bg-gray-100 p-0.5">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={o.value}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(o.value)}
            className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-md px-2.5 sm:px-3 h-8 text-sm transition-colors duration-150 ease-out-strong ${
              on ? 'bg-white text-gray-900 shadow-sm font-medium' : 'text-gray-600 hover:text-gray-900'
            }`}
          >
            {o.label}
            {o.count !== undefined && <span className={`tabular-nums text-xs ${on ? 'text-gray-500' : 'text-gray-400'}`}>{o.count}</span>}
          </button>
        );
      })}
    </div>
  );
}

const KIND_BADGE: Record<AssistantKind, string> = {
  agent: 'bg-indigo-50 text-indigo-700',
  skill: 'bg-emerald-50 text-emerald-700',
  rule: 'bg-amber-50 text-amber-800',
  command: 'bg-sky-50 text-sky-700',
  instructions: 'bg-gray-100 text-gray-700',
};

export function KindBadge({ kind }: { kind: AssistantKind }) {
  return <span className={`badge shrink-0 ${KIND_BADGE[kind] ?? KIND_BADGE.instructions}`}>{KIND_META[kind]?.label ?? kind}</span>;
}

/** A quiet tag for frontmatter values (tools, model, path globs). */
export function QuietTag({ children, mono = false, title }: { children: ReactNode; mono?: boolean; title?: string }) {
  return (
    <span
      title={title}
      className={`inline-block max-w-[18rem] truncate rounded px-1.5 py-0.5 text-xs text-gray-600 bg-gray-50 ring-1 ring-inset ring-gray-200 ${mono ? 'font-mono' : ''}`}
    >
      {children}
    </span>
  );
}

/** Frontmatter facts worth a glance in a list row, per kind. */
function rowTags(it: AssistantItem): { label: string; values: string[]; mono: boolean }[] {
  const fm = it.frontmatter;
  const out: { label: string; values: string[]; mono: boolean }[] = [];
  const model = fmString(fm, 'model');
  switch (it.kind) {
    case 'agent':
      if (model) out.push({ label: 'Model', values: [model], mono: false });
      out.push({ label: 'Tools', values: fmList(fm, 'tools'), mono: false });
      break;
    case 'skill':
      out.push({ label: 'Allowed tools', values: fmList(fm, 'allowed-tools', 'allowed_tools'), mono: false });
      break;
    case 'rule':
      out.push({ label: 'Applies to', values: fmList(fm, 'paths', 'globs', 'applyTo'), mono: true });
      break;
    case 'command':
      if (model) out.push({ label: 'Model', values: [model], mono: false });
      out.push({ label: 'Arguments', values: fmList(fm, 'argument-hint'), mono: true });
      out.push({ label: 'Allowed tools', values: fmList(fm, 'allowed-tools'), mono: false });
      break;
  }
  return out.filter((t) => t.values.length > 0);
}

const MAX_TAGS = 6;

function TagLine({ label, values, mono }: { label: string; values: string[]; mono: boolean }) {
  const shown = values.slice(0, MAX_TAGS);
  const more = values.length - shown.length;
  return (
    <div className="flex flex-wrap items-center gap-1 min-w-0">
      <span className="text-xs text-gray-500 mr-0.5">{label}</span>
      {shown.map((v) => (
        <QuietTag key={v} mono={mono} title={v}>
          {v}
        </QuietTag>
      ))}
      {more > 0 && (
        <span className="text-xs text-gray-500 tabular-nums" title={values.slice(MAX_TAGS).join(', ')}>
          +{more} more
        </span>
      )}
    </div>
  );
}

/** One row of an assistant list; the whole row links to the detail page. */
export function AssistantRow({ it, showKind }: { it: AssistantItem; showKind: boolean }) {
  const location = useLocation();
  const tags = rowTags(it);
  return (
    <li>
      <Link
        to={detailPath(it.product_id, it.code)}
        state={{ from: location.pathname + location.search }}
        className="kb-row grid gap-x-6 gap-y-1.5 px-4 py-3 md:grid-cols-[minmax(0,1fr)_minmax(8rem,14rem)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
      >
        <div className="min-w-0 space-y-1">
          <div className="flex items-center gap-2 min-w-0">
            <span className="font-medium text-gray-900 truncate" title={it.name}>
              {it.name || it.path}
            </span>
            {showKind && <KindBadge kind={it.kind} />}
          </div>
          {it.description ? (
            <p className="text-sm text-gray-600 line-clamp-2 break-words">{it.description}</p>
          ) : (
            <p className="text-sm text-gray-400">No description</p>
          )}
          {tags.map((t) => (
            <TagLine key={t.label} {...t} />
          ))}
          <div className="font-mono text-xs text-gray-500 truncate" title={it.path}>
            {it.path}
          </div>
        </div>
        <div className="text-sm text-gray-700 min-w-0 md:text-right">
          <span className="md:hidden text-gray-500">Product: </span>
          <span className="break-words">{it.product_name || `Product #${it.product_id}`}</span>
        </div>
      </Link>
    </li>
  );
}

export function RowsSkeleton({ rows = 6 }: { rows?: number }) {
  return (
    <ul aria-hidden="true" className="divide-y divide-gray-100">
      {Array.from({ length: rows }).map((_, i) => (
        <li key={i} className="px-4 py-3 space-y-2 motion-safe:animate-pulse">
          <div className="h-4 w-48 rounded bg-gray-200" />
          <div className="h-3 w-3/4 rounded bg-gray-100" />
          <div className="h-3 w-64 rounded bg-gray-100" />
        </li>
      ))}
    </ul>
  );
}

/** The empty state: says why there is nothing and how to get data. */
export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="px-4 py-12 text-center">
      <p className="font-medium text-gray-900">{title}</p>
      {children && <div className="mt-2 mx-auto max-w-xl text-sm text-gray-600 space-y-2 [&_code]:font-mono [&_code]:text-[0.85em] [&_code]:bg-gray-100 [&_code]:rounded [&_code]:px-1">{children}</div>}
    </div>
  );
}
