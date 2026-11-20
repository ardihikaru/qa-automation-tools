// /archive: archived AI conversations, unarchive, and "Prune all archived" for an Admin acting as
// itself (docs/designs/AI_CONVERSATIONS.md section 6).
import { useCallback, useEffect, useState } from 'react';
import { Trash2 } from 'lucide-react';
import { useAuth } from '../../auth/AuthContext';
import { usePageHeader } from '../../components/pageHeader';
import { absoluteTime, errorText, isAbort } from '../knowledge/api';
import { RelTime } from '../knowledge/ui';
import ConversationList from './ConversationList';
import PruneDialog from './PruneDialog';
import { ConversationPage, PruneEntry, canPrune, canReadMessages, formatBytes, listConversations, listPrunes } from './api';
import { plural } from '../history/api';

export default function ArchiveView() {
  const { user } = useAuth();
  const admin = canPrune(user);
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(25);
  const [data, setData] = useState<ConversationPage | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [key, setKey] = useState(0);
  const [prune, setPrune] = useState(false);
  const [log, setLog] = useState<PruneEntry[] | null>(null);
  const reload = useCallback(() => setKey((n) => n + 1), []);

  usePageHeader({ title: 'Archive' });

  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    listConversations({ archived: 1, page, page_size: size }, ac.signal)
      .then(setData)
      .catch((e) => !isAbort(e) && setError(errorText(e, 'Could not load the archive')))
      .finally(() => !ac.signal.aborted && setLoading(false));
    return () => ac.abort();
  }, [page, size, key]);

  useEffect(() => {
    if (!admin) return;
    const ac = new AbortController();
    listPrunes(ac.signal)
      .then((r) => setLog(r.items))
      .catch(() => undefined);
    return () => ac.abort();
  }, [admin, key]);

  const t = data?.totals;
  const nothing = !data || data.total === 0;

  return (
    <div className="space-y-4 min-w-0">
      <div className="card card-body flex flex-wrap items-center gap-x-6 gap-y-3">
        <p className="text-sm text-gray-700 min-w-0 flex-1">
          Archived AI conversations are hidden from the job pages and the run history. Unarchive one to bring it back. <strong>Prune all archived</strong> deletes their messages for good; job
          records, step times, the run history and AI cost records stay.
          {t && (
            <span className="block mt-1 text-gray-600 tabular-nums">
              Archived: {plural(t.archived.conversations, 'conversation')}, {formatBytes(t.archived.bytes)}. All stored: {plural(t.all.conversations, 'conversation')}, {formatBytes(t.all.bytes)}.
            </span>
          )}
        </p>
        <div className="flex flex-col items-end gap-1">
          <button
            type="button"
            className="btn btn-danger inline-flex items-center gap-1.5 h-9 px-3 text-sm disabled:opacity-50"
            disabled={!admin || nothing}
            onClick={() => setPrune(true)}
            aria-describedby={!admin ? 'prune-admins-only' : undefined}
          >
            <Trash2 className="w-4 h-4" aria-hidden="true" /> Prune all archived
          </button>
          {!admin && (
            <span id="prune-admins-only" className="text-xs text-gray-500">
              Admins only{user?.impersonator ? ' (not while impersonating)' : ''}
            </span>
          )}
        </div>
      </div>

      <ConversationList
        data={data}
        error={error}
        loading={loading}
        retry={reload}
        page={page}
        pageSize={size}
        onPage={setPage}
        onPageSize={(s) => {
          setSize(s);
          setPage(1);
        }}
        onChanged={reload}
        canAct={canReadMessages(user)}
        archived
      />

      {admin && log && log.length > 0 && (
        <section className="card" aria-labelledby="prune-log">
          <div className="card-header">
            <h2 id="prune-log" className="font-medium text-gray-900">
              Prune log
            </h2>
          </div>
          <ul className="divide-y divide-gray-100 text-sm">
            {log.map((e) => (
              <li key={e.id} className="px-4 py-2 flex flex-wrap gap-x-4 gap-y-1">
                <RelTime at={e.at} />
                <span>
                  {e.actor_name} pruned {plural(e.detail.conversations ?? 0, 'conversation')} ({plural(e.detail.messages ?? 0, 'message')}, {formatBytes(e.detail.bytes ?? 0)})
                </span>
                {e.detail.archived_before && <span className="text-gray-500">archived up to {absoluteTime(e.detail.archived_before)}</span>}
                {e.detail.error && <span className="text-red-700">{e.detail.error}</span>}
              </li>
            ))}
          </ul>
        </section>
      )}

      {prune && <PruneDialog onClose={() => setPrune(false)} onPruned={reload} />}
    </div>
  );
}
