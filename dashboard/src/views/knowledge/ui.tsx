// Small presentational pieces shared by the knowledge card, view and drawer.
import { ReactNode, useState } from 'react';
import { AlertCircle, ChevronLeft, ChevronRight, Loader2 } from 'lucide-react';
import { AiStatus, Diff, DiffKind, Problems, SyncStatus, absoluteTime, commitUrl, relativeTime, shortSha } from './api';

const RELEASE_CLASS: Record<string, string> = {
  PROD: 'bg-green-100 text-green-800',
  'DEV-DONE': 'bg-blue-100 text-blue-800',
  'IN-PROGRESS': 'bg-amber-100 text-amber-800',
  'NOT-STARTED': 'bg-gray-100 text-gray-700',
  UNKNOWN: 'bg-white text-gray-500 ring-1 ring-inset ring-gray-300',
};

export function ReleaseChip({ status, asOf }: { status: string; asOf?: string }) {
  return (
    <span className={`badge whitespace-nowrap ${RELEASE_CLASS[status] ?? RELEASE_CLASS.UNKNOWN}`} title={asOf ? `Status as of ${asOf}` : undefined}>
      {status || 'UNKNOWN'}
    </span>
  );
}

const CASE_STATUS_CLASS: Record<string, string> = {
  automated: 'bg-green-100 text-green-800',
  planned: 'bg-blue-50 text-blue-800',
  blocked: 'bg-red-100 text-red-800',
  'manual-only': 'bg-purple-100 text-purple-800',
  wont: 'bg-gray-100 text-gray-600',
};

export function CaseStatusChip({ status }: { status: string }) {
  return <span className={`badge whitespace-nowrap ${CASE_STATUS_CLASS[status] ?? 'bg-gray-100 text-gray-700'}`}>{status}</span>;
}

const SYNC_CLASS: Record<SyncStatus, string> = {
  running: 'bg-blue-100 text-blue-800',
  succeeded: 'badge-success',
  failed: 'badge-error',
  skipped: 'bg-gray-100 text-gray-700',
};

export function SyncStatusChip({ status }: { status: SyncStatus }) {
  return <span className={`badge whitespace-nowrap ${SYNC_CLASS[status] ?? 'bg-gray-100 text-gray-700'}`}>{status}</span>;
}

export function AiStatusNote({ status, error }: { status: AiStatus; error: string | null }) {
  if (status === 'pending') {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-gray-500">
        <Loader2 className="w-3.5 h-3.5 motion-safe:animate-spin" aria-hidden="true" /> Summarising…
      </span>
    );
  }
  if (status === 'failed') {
    return (
      <span className="text-xs text-gray-400 break-words" title={error ?? undefined}>
        AI summary unavailable{error ? `: ${error}` : ''}
      </span>
    );
  }
  return null;
}

export function LocalBadge() {
  return (
    <span className="badge bg-violet-100 text-violet-800 shrink-0" title="Local draft: lives only in this app">
      Local
    </span>
  );
}

export function RemovedBadge({ syncId }: { syncId?: number | null }) {
  return (
    <span className="badge bg-red-50 text-red-700 shrink-0 whitespace-nowrap" title="No longer in the automation repo">
      {syncId ? `removed in sync #${syncId}` : 'removed'}
    </span>
  );
}

/** Code + optional markers for an entity row. */
export function CodeLabel({ code, origin, removed }: { code: string; origin: string; removed: boolean }) {
  return (
    <span className="inline-flex items-center gap-1.5 shrink-0">
      <span className={`font-mono text-xs whitespace-nowrap ${removed ? 'line-through text-gray-400' : 'text-gray-800'}`}>{code}</span>
      {origin === 'local' && <LocalBadge />}
    </span>
  );
}

export function CoverageBar({ value, className = '' }: { value: number; className?: string }) {
  const v = Math.max(0, Math.min(100, value || 0));
  return (
    <span className={`inline-flex items-center gap-2 ${className}`} title={`${v.toFixed(1)}% automated`}>
      <span className="relative block h-1.5 w-20 rounded-full bg-gray-200 overflow-hidden" aria-hidden="true">
        <span
          className={`absolute inset-y-0 left-0 rounded-full ${v >= 80 ? 'bg-green-500' : v >= 40 ? 'bg-amber-500' : 'bg-red-400'}`}
          style={{ width: `${v}%` }}
        />
      </span>
      <span className="text-xs tabular-nums text-gray-600 w-9 text-right">{Math.round(v)}%</span>
    </span>
  );
}

export function RelTime({ at }: { at: string | null | undefined }) {
  if (!at) return null;
  return (
    <time dateTime={at} title={absoluteTime(at)} className="whitespace-nowrap">
      {relativeTime(at)}
    </time>
  );
}

export function ShaLink({ sha, repoUrl }: { sha: string | null | undefined; repoUrl: string | null | undefined }) {
  if (!sha) return null;
  const url = commitUrl(repoUrl, sha);
  const label = <code className="text-xs">{shortSha(sha)}</code>;
  return url ? (
    <a href={url} target="_blank" rel="noreferrer" className="text-primary-600 hover:underline" title={sha}>
      {label}
    </a>
  ) : (
    <span title={sha}>{label}</span>
  );
}

const DIFF_SIGNS: { kind: DiffKind; sign: string; cls: string; label: string }[] = [
  { kind: 'added', sign: '+', cls: 'text-green-700', label: 'added' },
  { kind: 'updated', sign: '~', cls: 'text-amber-700', label: 'updated' },
  { kind: 'removed', sign: '−', cls: 'text-red-700', label: 'removed' },
  { kind: 'restored', sign: '↺', cls: 'text-blue-700', label: 'restored' },
];

export function DiffTotals({ diff }: { diff: Diff | null | undefined }) {
  if (!diff) return null;
  return (
    <span className="inline-flex flex-wrap gap-x-3 gap-y-1 text-xs tabular-nums">
      {DIFF_SIGNS.map((d) => (
        <span key={d.kind} className={diff.total?.[d.kind] ? d.cls : 'text-gray-400'} title={d.label}>
          {d.sign}
          {diff.total?.[d.kind] ?? 0}
          <span className="sr-only"> {d.label}</span>
        </span>
      ))}
    </span>
  );
}

/** Renders an AI summary: "- " lines become a list, the rest are paragraphs. No markdown dependency. */
export function AiSummaryText({ text }: { text: string }) {
  const blocks: ReactNode[] = [];
  let list: string[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) {
      blocks.push(
        <p key={`p${blocks.length}`} className="break-words">
          {para.join(' ')}
        </p>,
      );
      para = [];
    }
    if (list.length) {
      blocks.push(
        <ul key={`l${blocks.length}`} className="list-disc pl-5 space-y-0.5">
          {list.map((l, i) => (
            <li key={i} className="break-words">
              {l}
            </li>
          ))}
        </ul>,
      );
      list = [];
    }
  };
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    const m = line.match(/^[-*]\s+(.*)$/);
    if (m) {
      if (para.length) flush();
      list.push(m[1].replace(/\*\*/g, ''));
    } else if (!line) {
      flush();
    } else {
      if (list.length) flush();
      para.push(line.replace(/^#+\s*/, '').replace(/\*\*/g, ''));
    }
  }
  flush();
  return <div className="space-y-2 text-sm text-gray-700">{blocks}</div>;
}

export function ProblemsList({ problems }: { problems: Problems | null | undefined }) {
  const [open, setOpen] = useState(false);
  if (!problems || !problems.total) return <p className="text-xs text-gray-500">No parse problems.</p>;
  const msgs = Object.entries(problems.by_message ?? {}).sort((a, b) => b[1] - a[1]);
  const items = problems.items ?? [];
  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-amber-800 inline-flex items-center gap-1">
        <AlertCircle className="w-3.5 h-3.5" aria-hidden="true" /> {problems.total} parse problem{problems.total === 1 ? '' : 's'}
      </div>
      <ul className="text-xs text-gray-700 space-y-0.5">
        {msgs.map(([m, n]) => (
          <li key={m} className="flex gap-2 min-w-0">
            <span className="tabular-nums text-gray-500 w-8 shrink-0 text-right">{n}×</span>
            <span className="break-words min-w-0">{m}</span>
          </li>
        ))}
      </ul>
      {items.length > 0 && (
        <div>
          <button type="button" className="text-xs text-primary-600 hover:underline min-h-8" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
            {open ? 'Hide' : 'Show'} {items.length} item{items.length === 1 ? '' : 's'}
            {problems.total > items.length ? ` (first ${items.length} of ${problems.total})` : ''}
          </button>
          {open && (
            <ul className="mt-1 max-h-64 overflow-auto rounded border border-gray-200 bg-gray-50 p-2 text-xs font-mono space-y-1">
              {items.map((p, i) => (
                <li key={i} className="break-words">
                  <span className="text-gray-500">
                    {p.file || '(catalog)'}
                    {p.line ? `:${p.line}` : ''}
                  </span>{' '}
                  {p.msg}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

/** A disclosure with a rotating caret (transform only). */
export function Disclosure({ summary, children, defaultOpen = false }: { summary: ReactNode; children: ReactNode; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div>
      <button type="button" className="inline-flex items-center gap-1 text-sm font-medium text-gray-800 min-h-8" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <ChevronRight className={`w-4 h-4 text-gray-500 kb-caret ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
        {summary}
      </button>
      {open && <div className="mt-2">{children}</div>}
    </div>
  );
}

export function ErrorNote({ children, onRetry }: { children: ReactNode; onRetry?: () => void }) {
  return (
    <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2 flex items-start justify-between gap-3 break-words">
      <span className="min-w-0">{children}</span>
      {onRetry && (
        <button type="button" className="text-red-700 underline shrink-0" onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  );
}

export function Loading({ label = 'Loading…' }: { label?: string }) {
  return (
    <div role="status" className="flex items-center gap-2 text-sm text-gray-500 py-6 justify-center">
      <Loader2 className="w-4 h-4 motion-safe:animate-spin" aria-hidden="true" /> {label}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="text-sm text-gray-500 py-10 text-center">{children}</div>;
}

export function Pagination({ page, pageSize, total, onPage, disabled }: { page: number; pageSize: number; total: number; onPage: (p: number) => void; disabled?: boolean }) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = Math.min(total, page * pageSize);
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-gray-600">
      <span className="tabular-nums">
        {from}–{to} of {total}
      </span>
      <div className="flex items-center gap-2">
        <button type="button" className="btn btn-secondary px-2 py-1 min-h-8 disabled:opacity-50" disabled={disabled || page <= 1} onClick={() => onPage(page - 1)} aria-label="Previous page">
          <ChevronLeft className="w-4 h-4" aria-hidden="true" />
        </button>
        <span className="tabular-nums">
          Page {page} of {pages}
        </span>
        <button type="button" className="btn btn-secondary px-2 py-1 min-h-8 disabled:opacity-50" disabled={disabled || page >= pages} onClick={() => onPage(page + 1)} aria-label="Next page">
          <ChevronRight className="w-4 h-4" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}
