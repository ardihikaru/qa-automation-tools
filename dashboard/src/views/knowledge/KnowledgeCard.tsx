import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowRight, BookOpen, ChevronRight, RefreshCw } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import { Product, api } from '../products/api';
import { Summary, SyncRow, errorText, getSummary, getSync, shortSha, startSync } from './api';
import { AiStatusNote, AiSummaryText, DiffTotals, ErrorNote, ProblemsList, RelTime, ShaLink, SyncStatusChip } from './ui';
import { selectCls } from './styles';
import { useSyncFollower } from './useSyncFollower';

interface Props {
  product: Product;
  /** Admin, DevOps, Developer, QA, QC: Sync, notes and drafts. */
  canWrite: boolean;
  /** Admin and DevOps: set the catalog format. */
  canSetFormat: boolean;
  onProductChange: (p: Product) => void;
}

function Tile({ label, value, sub }: { label: string; value: string | number; sub?: string }) {
  return (
    <div className="rounded-lg border border-gray-200 px-3 py-2 min-w-0">
      <div className="text-xs text-gray-500 truncate">{label}</div>
      <div className="text-xl font-semibold tabular-nums text-gray-900">{value}</div>
      {sub && <div className="text-xs text-gray-500 truncate" title={sub}>{sub}</div>}
    </div>
  );
}

export default function KnowledgeCard({ product, canWrite, canSetFormat, onProductChange }: Props) {
  const pid = product.id;
  const autoRepoId = product.repos?.find((r) => r.is_automation)?.id ?? 0;
  const [summary, setSummary] = useState<Summary | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [syncNote, setSyncNote] = useState<string | null>(null);
  const inflight = useRef(false);
  const [format, setFormat] = useState('');
  const [savingFormat, setSavingFormat] = useState(false);
  const [formatError, setFormatError] = useState<string | null>(null);
  const [changesOpen, setChangesOpen] = useState(false);
  const [full, setFull] = useState<SyncRow | null>(null);
  const [fullError, setFullError] = useState<string | null>(null);
  const loadSeq = useRef(0);

  const load = useCallback(
    async (quiet = false) => {
      const seq = ++loadSeq.current;
      if (!quiet) setLoading(true);
      try {
        const s = await getSummary(pid);
        if (seq !== loadSeq.current) return;
        setSummary(s);
        setLoadError(null);
      } catch (e) {
        if (seq !== loadSeq.current) return;
        if (!quiet) setLoadError(errorText(e, 'Could not load the knowledge base'));
      } finally {
        if (seq === loadSeq.current) setLoading(false);
      }
    },
    [pid],
  );

  const follower = useSyncFollower(pid, (s, statusChanged) => {
    if (statusChanged || (s.status !== 'running' && s.ai_status !== 'pending')) void load(true);
  });
  const { follow, following } = follower;

  useEffect(() => {
    void load();
  }, [load, product.knowledge_format, autoRepoId]);

  // Resume polling when the page opens on a running sync (or one whose summary is still pending).
  const latestId = summary?.latest_sync?.id;
  const latestLive = summary?.latest_sync ? summary.latest_sync.status === 'running' || summary.latest_sync.ai_status === 'pending' : false;
  useEffect(() => {
    if (latestId && latestLive && !following && !follower.timedOut && follower.sync?.id !== latestId) follow(latestId);
  }, [latestId, latestLive, following, follow, follower.sync?.id, follower.timedOut]);

  useEffect(() => {
    if (summary && !format) setFormat(summary.formats[0] ?? '');
  }, [summary, format]);

  const fromSummary = summary?.latest_sync ?? null;
  const latest = follower.sync && (!fromSummary || follower.sync.id >= fromSummary.id) ? follower.sync : fromSummary;
  const running = latest?.status === 'running';
  const changeSync = latest && latest.status === 'succeeded' ? latest : summary?.last_success ?? null;
  const changeFull = changeSync && follower.sync?.id === changeSync.id ? follower.sync : full?.id === changeSync?.id ? full : null;

  // Problem items and code lists only come with GET syncs/{id}: load them when the disclosure opens.
  const changeId = changeSync?.id;
  const haveFull = !!changeFull;
  useEffect(() => {
    if (!changesOpen || !changeId || haveFull) return;
    const ac = new AbortController();
    setFullError(null);
    getSync(pid, changeId, ac.signal)
      .then(setFull)
      .catch((e) => !ac.signal.aborted && setFullError(errorText(e)));
    return () => ac.abort();
  }, [changesOpen, changeId, haveFull, pid]);

  async function onSync() {
    if (inflight.current || running) return;
    inflight.current = true;
    setStarting(true);
    setSyncError(null);
    setSyncNote(null);
    try {
      const r = await startSync(pid);
      if (r.attached) setSyncNote(`Sync #${r.sync_id} was already running; following it.`);
      follow(r.sync_id);
      void load(true);
    } catch (e) {
      setSyncError(errorText(e, 'Could not start the sync'));
    } finally {
      inflight.current = false;
      setStarting(false);
    }
  }

  async function saveFormat() {
    if (savingFormat || !format) return;
    setSavingFormat(true);
    setFormatError(null);
    try {
      const p = await api<Product>(`/api/products/${pid}`, { method: 'PUT', body: JSON.stringify({ knowledge_format: format }) });
      onProductChange(p);
      await load(true);
    } catch (e) {
      setFormatError(errorText(e, 'Could not save the format'));
    } finally {
      setSavingFormat(false);
    }
  }

  const header = (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <h3 className="text-base font-semibold inline-flex items-center gap-2">
        <BookOpen className="w-5 h-5 text-primary-600" aria-hidden="true" /> Knowledge base
      </h3>
      {summary?.configured && (
        <Link to={`/products/${pid}/knowledge`} className="text-sm text-primary-600 hover:underline inline-flex items-center gap-1 min-h-8">
          Open knowledge base <ArrowRight className="w-4 h-4" aria-hidden="true" />
        </Link>
      )}
    </div>
  );

  if (loading && !summary) {
    return (
      <section className="card p-4 space-y-3" aria-label="Knowledge base">
        {header}
        <div className="grid gap-2 grid-cols-2 sm:grid-cols-3 xl:grid-cols-5" aria-hidden="true">
          {[0, 1, 2, 3, 4].map((i) => (
            <div key={i} className="h-[66px] rounded-lg bg-gray-100 motion-safe:animate-pulse" />
          ))}
        </div>
      </section>
    );
  }

  if (!summary) {
    return (
      <section className="card p-4 space-y-3" aria-label="Knowledge base">
        {header}
        <ErrorNote onRetry={() => void load()}>{loadError ?? 'Could not load the knowledge base'}</ErrorNote>
      </section>
    );
  }

  if (!summary.configured) {
    return (
      <section className="card p-4 space-y-3" aria-label="Knowledge base">
        {header}
        <p className="text-sm text-gray-600">
          {summary.format ? (
            <>
              Unknown catalog format <code>{summary.format}</code>.
            </>
          ) : (
            'No catalog format set.'
          )}{' '}
          {!canSetFormat && 'An Admin or DevOps user can set one.'}
        </p>
        {canSetFormat && (
          <div className="flex flex-wrap items-center gap-2">
            <label className="text-sm text-gray-700" htmlFor={`kb-format-${pid}`}>
              Catalog format
            </label>
            <select id={`kb-format-${pid}`} className={selectCls} value={format} onChange={(e) => setFormat(e.target.value)} disabled={savingFormat}>
              {summary.formats.length === 0 && <option value="">No formats available</option>}
              {summary.formats.map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
            </select>
            <SubmitButton className="btn btn-primary text-sm" loading={savingFormat} loadingLabel="Saving…" disabled={!format} onClick={saveFormat}>
              Save
            </SubmitButton>
          </div>
        )}
        {formatError && <ErrorNote>{formatError}</ErrorNote>}
      </section>
    );
  }

  const c = summary.counts;
  const busy = starting || running || follower.waiting;
  const noRepo = !summary.repo;
  const syncTitle = !canWrite ? 'Your role cannot sync the knowledge base' : noRepo ? 'Set an automation repo first' : undefined;

  return (
    <section className="card p-4 space-y-4" aria-label="Knowledge base">
      {header}

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
        <button
          type="button"
          className="btn btn-primary text-sm w-32 inline-flex items-center justify-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
          aria-busy={busy}
          disabled={busy || !canWrite || noRepo}
          title={syncTitle}
          onClick={onSync}
        >
          <RefreshCw className={`w-4 h-4 ${busy ? 'motion-safe:animate-spin' : ''}`} aria-hidden="true" />
          {busy ? 'Syncing…' : 'Sync now'}
        </button>
        {latest ? (
          <span className="inline-flex flex-wrap items-center gap-x-2 gap-y-1 text-gray-600 min-w-0">
            <SyncStatusChip status={latest.status} />
            <span>
              Sync #{latest.id} <RelTime at={latest.finished_at ?? latest.started_at} />
            </span>
            <ShaLink sha={latest.commit_sha} repoUrl={summary.repo?.url ?? latest.repo_url} />
            {latest.trigger_kind === 'schedule' && <span className="text-xs text-gray-400">scheduled</span>}
          </span>
        ) : (
          <span className="text-gray-500">Never synced.</span>
        )}
        {summary.repo && (
          <span className="text-xs text-gray-500 truncate min-w-0" title={`${summary.repo.url} (${summary.repo.branch})`}>
            from {summary.repo.name} · {summary.repo.branch} · {summary.format}
          </span>
        )}
      </div>

      {noRepo && <p className="text-sm text-yellow-800 bg-yellow-50 border border-yellow-100 rounded-lg px-3 py-2">No automation repo: the knowledge base syncs from the automation repo.</p>}
      {syncNote && <p role="status" className="text-sm text-blue-800 bg-blue-50 border border-blue-100 rounded-lg px-3 py-2">{syncNote}</p>}
      {syncError && <ErrorNote>{syncError}</ErrorNote>}
      {latest?.status === 'failed' && latest.error && <ErrorNote>Sync #{latest.id} failed: {latest.error}</ErrorNote>}
      {follower.timedOut && <p className="text-xs text-gray-500">Stopped following after 10 minutes; reload the page to check again.</p>}

      <div className="grid gap-2 grid-cols-2 sm:grid-cols-3 xl:grid-cols-5">
        <Tile label="Modules" value={c.modules} sub={`${c.parts} parts · ${c.submodules} sub-modules`} />
        <Tile label="Features" value={c.features} sub={`${c.features_with_stories} with stories`} />
        <Tile label="Stories" value={c.stories} sub={`${c.stories_unmapped} unmapped${summary.local.stories ? ` · ${summary.local.stories} local` : ''}`} />
        <Tile label="Test cases" value={c.test_cases} sub={`${c.epics} epics${summary.local.test_cases ? ` · ${summary.local.test_cases} local` : ''}`} />
        <Tile label="Automated" value={`${Math.round(summary.coverage.pct)}%`} sub={`${summary.coverage.automated} of ${summary.coverage.test_cases}`} />
      </div>

      {changeSync && (
        <div className="border-t border-gray-100 pt-3">
          <button
            type="button"
            className="inline-flex items-center gap-1 text-sm font-medium text-gray-800 min-h-8"
            aria-expanded={changesOpen}
            onClick={() => setChangesOpen((o) => !o)}
          >
            <ChevronRight className={`w-4 h-4 text-gray-500 kb-caret ${changesOpen ? 'rotate-90' : ''}`} aria-hidden="true" />
            What changed
            <span className="font-normal text-gray-500 ml-1">(sync #{changeSync.id})</span>
            <span className="ml-2">
              {changeSync.diff?.unchanged ? <span className="text-xs font-normal text-gray-500">up to date</span> : <DiffTotals diff={changeSync.diff} />}
            </span>
          </button>
          {changesOpen && (
            <div className="mt-2 grid gap-4 lg:grid-cols-2">
              <div className="space-y-2 min-w-0">
                {changeSync.diff?.unchanged ? (
                  <p className="text-sm text-gray-600">Already up to date ({shortSha(changeSync.commit_sha)}).</p>
                ) : (
                  <>
                    <DiffTotals diff={changeSync.diff} />
                    {changeSync.ai_summary ? <AiSummaryText text={changeSync.ai_summary} /> : null}
                  </>
                )}
                <AiStatusNote status={(changeFull ?? changeSync).ai_status} error={(changeFull ?? changeSync).ai_error} />
              </div>
              <div className="min-w-0">
                {fullError && <ErrorNote>{fullError}</ErrorNote>}
                <ProblemsList problems={(changeFull ?? changeSync).problems} />
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  );
}
