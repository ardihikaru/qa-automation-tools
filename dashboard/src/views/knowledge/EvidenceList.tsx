import { EvidenceItem, SyncRef } from './api';

const KIND_CLASS: Record<string, string> = {
  GIT: 'bg-slate-100 text-slate-700 border-slate-200',
  SRV: 'bg-amber-50 text-amber-800 border-amber-200',
  DOC: 'bg-sky-50 text-sky-800 border-sky-200',
  RN: 'bg-emerald-50 text-emerald-800 border-emerald-200',
};

/** The evidence of a feature row as nodes: kind, what it is, date, and a link when there is one. */
export function EvidenceList({ items, raw }: { items?: EvidenceItem[]; raw?: string }) {
  if (!items) return raw ? <span>{raw}</span> : null;
  if (items.length === 0) return null;
  return (
    <ul className="space-y-1">
      {items.map((it, i) => (
        <li key={`${it.raw}-${i}`} className="flex flex-wrap items-center gap-2 text-sm">
          <span className={`inline-block rounded border px-1.5 py-0.5 font-mono text-[11px] leading-none ${KIND_CLASS[it.kind] ?? 'bg-gray-50 text-gray-700 border-gray-200'}`}>{it.kind}</span>
          {it.url ? (
            <a href={it.url} target="_blank" rel="noopener noreferrer" className="text-teal-700 underline break-all">
              {it.label}
            </a>
          ) : (
            <span className="break-all">{it.label}</span>
          )}
          {it.date && <span className="text-xs text-gray-500 tabular-nums">{it.date}</span>}
        </li>
      ))}
    </ul>
  );
}

/** "2026-11-26 (sync #77, catalogue d718d17)" for the sync that last changed a row. */
export function SyncStamp({ s }: { s?: SyncRef | null }) {
  if (!s) return null;
  const day = s.finished_at ? s.finished_at.slice(0, 10) : '';
  return (
    <span className="tabular-nums">
      {day}
      <span className="text-xs text-gray-500">
        {' '}
        (sync #{s.id}
        {s.commit_sha ? `, catalogue ${s.commit_sha.slice(0, 7)}` : ''})
      </span>
    </span>
  );
}
