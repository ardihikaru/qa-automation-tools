import { useEffect, useState } from 'react';
import { ChevronRight, RefreshCw } from 'lucide-react';
import { DiffKind, EntityType, SyncRow, TYPE_LABEL, duration, errorText, getSync, isEntityType, listSyncs } from './api';
import { useLoad } from './hooks';
import { AiStatusNote, AiSummaryText, DiffTotals, Empty, ErrorNote, Loading, ProblemsList, RelTime, ShaLink, SyncStatusChip } from './ui';
import { Target } from './EntityPanel';

interface Props {
  pid: number;
  refreshKey: number;
  onOpen: (t: Target) => void;
}

const KINDS: DiffKind[] = ['added', 'updated', 'removed', 'restored'];

function SyncDetail({ pid, row, onOpen }: { pid: number; row: SyncRow; onOpen: (t: Target) => void }) {
  const [full, setFull] = useState<SyncRow | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const ac = new AbortController();
    getSync(pid, row.id, ac.signal)
      .then(setFull)
      .catch((e) => !ac.signal.aborted && setError(errorText(e)));
    return () => ac.abort();
  }, [pid, row.id, row.status, row.ai_status]);

  const s = full ?? row;
  const byType = s.diff?.by_type ?? {};
  const types = (Object.keys(byType) as string[]).filter(isEntityType) as EntityType[];

  return (
    <div className="grid gap-4 xl:grid-cols-3 px-4 pb-4 pt-1">
      <div className="space-y-2 min-w-0">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">Summary</h4>
        {s.error && <ErrorNote>{s.error}</ErrorNote>}
        {s.diff?.unchanged && <p className="text-sm text-gray-600">Already up to date.</p>}
        {s.ai_summary ? <AiSummaryText text={s.ai_summary} /> : s.ai_status === 'none' && !s.error && !s.diff?.unchanged && <p className="text-sm text-gray-500">No AI summary.</p>}
        <AiStatusNote status={s.ai_status} error={s.ai_error} />
        <dl className="grid grid-cols-[7rem_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs text-gray-600">
          <dt>Repo</dt>
          <dd className="truncate" title={s.repo_url}>
            {s.repo_name} · {s.branch}
          </dd>
          <dt>Format</dt>
          <dd>
            {s.format} v{s.parser_version}
          </dd>
          {s.triggered_by && (
            <>
              <dt>Triggered by</dt>
              <dd className="truncate font-mono" title={s.triggered_by}>
                {s.triggered_by}
              </dd>
            </>
          )}
        </dl>
      </div>
      <div className="space-y-2 min-w-0">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">Changed entities</h4>
        {error && <ErrorNote>{error}</ErrorNote>}
        {!full && !error && <Loading />}
        {full && types.length === 0 && <p className="text-sm text-gray-500">No entity changed.</p>}
        {full &&
          types.map((t) => (
            <div key={t} className="text-sm">
              <div className="font-medium text-gray-800">{TYPE_LABEL[t]}</div>
              {KINDS.filter((k) => byType[t]?.[k]?.length).map((k) => (
                <div key={k} className="pl-2">
                  <span className="text-xs text-gray-500">
                    {k} ({byType[t]![k]!.length})
                  </span>
                  <div className="flex flex-wrap gap-1 mt-0.5 max-h-40 overflow-y-auto">
                    {byType[t]![k]!.map((code) => (
                      <button key={code} type="button" className="badge bg-gray-100 text-gray-700 font-mono hover:bg-gray-200" onClick={() => onOpen({ type: t, code })}>
                        {code}
                      </button>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          ))}
        {full?.diff?.truncated && <p className="text-xs text-gray-500">Lists are cut at 200 codes per kind.</p>}
      </div>
      <div className="min-w-0 space-y-2">
        <h4 className="text-xs font-semibold uppercase tracking-wide text-gray-500">Parse problems</h4>
        <ProblemsList problems={s.problems} />
      </div>
    </div>
  );
}

export default function SyncsTab({ pid, refreshKey, onOpen }: Props) {
  const [tick, setTick] = useState(0);
  const { data, loading, error, reload } = useLoad<SyncRow[]>(`${pid}:${refreshKey}:${tick}`, (s) => listSyncs(pid, 50, s));
  const [open, setOpen] = useState<number | null>(null);
  const live = data?.some((s) => s.status === 'running' || s.ai_status === 'pending');

  // Refresh the history every 5 s while a sync is running or summarising.
  useEffect(() => {
    if (!live) return;
    const t = window.setTimeout(() => setTick((n) => n + 1), 5000);
    return () => window.clearTimeout(t);
  }, [live, data]);

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-gray-500">Latest 50 syncs. Scheduled syncs run every 6 hours when the repo changed.</p>
        <button type="button" className="btn btn-secondary text-sm py-1 min-h-8 inline-flex items-center gap-1.5" onClick={reload} disabled={loading}>
          <RefreshCw className={`w-4 h-4 ${loading ? 'motion-safe:animate-spin' : ''}`} aria-hidden="true" /> Refresh
        </button>
      </div>
      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <Loading />}
      {data && data.length === 0 && <Empty>No syncs yet.</Empty>}
      {data && data.length > 0 && (
        <div className="card overflow-hidden">
          <ul className="divide-y divide-gray-100">
            {data.map((s) => {
              const isOpen = open === s.id;
              return (
                <li key={s.id}>
                  <div className="kb-row flex items-center">
                  <button
                    type="button"
                    className="flex-1 min-w-0 text-left grid grid-cols-[1.25rem_4rem_6rem_minmax(0,1fr)] md:grid-cols-[1.25rem_4rem_6rem_6rem_8rem_5rem_minmax(0,1fr)] items-center gap-x-3 gap-y-1 px-4 py-2.5 min-h-8 text-sm"
                    aria-expanded={isOpen}
                    onClick={() => setOpen(isOpen ? null : s.id)}
                  >
                    <ChevronRight className={`w-4 h-4 text-gray-500 kb-caret ${isOpen ? 'rotate-90' : ''}`} aria-hidden="true" />
                    <span className="tabular-nums text-gray-700">#{s.id}</span>
                    <span>
                      <SyncStatusChip status={s.status} />
                    </span>
                    <span className="text-xs text-gray-500 md:order-none">{s.trigger_kind}</span>
                    <span className="hidden md:block text-gray-600">
                      <RelTime at={s.started_at} />
                    </span>
                    <span className="hidden md:block text-xs tabular-nums text-gray-500">{duration(s.started_at, s.finished_at) || (s.status === 'running' ? '…' : '')}</span>
                    <span className="hidden md:flex items-center gap-3 min-w-0">
                      {s.diff?.unchanged ? <span className="text-xs text-gray-500">up to date</span> : <DiffTotals diff={s.diff} />}
                      {s.problems && s.problems.total > 0 && <span className="text-xs text-amber-700">{s.problems.total} problems</span>}
                    </span>
                  </button>
                  <span className="shrink-0 pr-4 pl-2 w-20 text-right">
                    <ShaLink sha={s.commit_sha} repoUrl={s.repo_url} />
                  </span>
                  </div>
                  {isOpen && <SyncDetail pid={pid} row={s} onOpen={onOpen} />}
                </li>
              );
            })}
          </ul>
        </div>
      )}
    </div>
  );
}
