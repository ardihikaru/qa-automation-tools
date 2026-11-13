// The "Knowledge base" cell of a /products row: how much of the catalog is automated, how fresh
// the last sync is, a Sync shortcut and a link to the knowledge base. Each row loads its own
// summary after the table renders, so a slow or failing product never blocks the list.
import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertCircle, BookOpen } from 'lucide-react';
import { Product } from './api';
import { Summary, absoluteTime, errorText, getSummary, knowledgeHref, relativeTime } from '../knowledge/api';
import { useKnowledgeSync } from '../knowledge/useKnowledgeSync';
import SyncButton from '../knowledge/SyncButton';

interface Props {
  product: Product;
  /** Admin, DevOps, Developer, QA, QC. */
  canSync: boolean;
  /** Admin and DevOps: may set the catalog format (in the product details). */
  canSetUp: boolean;
  onOpenDetails: () => void;
}

export default function KnowledgeCell({ product, canSync, canSetUp, onOpenDetails }: Props) {
  if (!product.knowledge_format) {
    return (
      <div className="text-sm text-gray-400">
        Not set up
        {canSetUp && (
          <>
            {' · '}
            <button
              type="button"
              onClick={onOpenDetails}
              className="text-primary-600 hover:underline underline-offset-2 rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
            >
              Set up in details
            </button>
          </>
        )}
      </div>
    );
  }
  return <ConfiguredCell key={`${product.id}:${product.knowledge_format}`} product={product} canSync={canSync} />;
}

/** Automated share as a thin two-part bar: automated in the case grid's teal, the rest grey. */
function ShareBar({ automated, total }: { automated: number; total: number }) {
  if (total === 0) return <span aria-hidden="true" className="kb-bar-empty block w-24 shrink-0" />;
  return (
    <span aria-hidden="true" className="kb-bar w-24 shrink-0">
      {automated > 0 && <span data-s="automated" className="kb-st" style={{ flexGrow: automated, flexBasis: 0 }} />}
      {total - automated > 0 && <span className="kb-st" style={{ flexGrow: total - automated, flexBasis: 0 }} />}
    </span>
  );
}

function ConfiguredCell({ product, canSync }: { product: Product; canSync: boolean }) {
  const pid = product.id;
  const [summary, setSummary] = useState<Summary | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const seq = useRef(0);

  const load = useCallback(
    async (quiet = false) => {
      const n = ++seq.current;
      if (!quiet) setLoading(true);
      try {
        const s = await getSummary(pid);
        if (n !== seq.current) return;
        setSummary(s);
        setLoadError(null);
      } catch (e) {
        if (n !== seq.current) return;
        // A failed background refresh keeps the last summary on screen.
        if (!quiet) setLoadError(errorText(e, 'Could not load'));
      } finally {
        if (n === seq.current) setLoading(false);
      }
    },
    [pid],
  );

  useEffect(() => {
    void load();
  }, [load]);

  const sync = useKnowledgeSync(pid, summary?.latest_sync ?? null, () => void load(true));

  const openLink = (
    <Link
      to={knowledgeHref(pid)}
      className="inline-flex items-center gap-1.5 h-8 px-1 text-sm text-primary-600 hover:underline underline-offset-2 whitespace-nowrap rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
    >
      <BookOpen className="w-4 h-4 shrink-0" aria-hidden="true" />
      Knowledge base
    </Link>
  );

  let body;
  if (!summary) {
    body = loading ? (
      <div aria-hidden="true" className="space-y-1.5 py-1">
        <div className="h-2 w-24 rounded bg-gray-100 motion-safe:animate-pulse" />
        <div className="h-3 w-32 rounded bg-gray-100 motion-safe:animate-pulse" />
      </div>
    ) : (
      <p role="alert" className="text-xs text-red-700 inline-flex items-start gap-1">
        <AlertCircle className="w-3.5 h-3.5 mt-px shrink-0" aria-hidden="true" />
        <span className="min-w-0 break-words">{loadError ?? 'Could not load'}</span>
        <button type="button" className="underline shrink-0" onClick={() => void load()}>
          Retry
        </button>
      </p>
    );
  } else if (!summary.configured) {
    body = (
      <p className="text-sm text-gray-500">
        Unknown format <code className="text-xs">{summary.format}</code>
      </p>
    );
  } else {
    const { automated, test_cases: total } = summary.coverage;
    const latest = sync.latest;
    const at = latest?.finished_at ?? latest?.started_at;
    const lastOk = summary.last_success;
    let fresh;
    if (sync.busy) {
      fresh = <span className="text-gray-700">Syncing…</span>;
    } else if (latest?.status === 'failed') {
      fresh = (
        <span className="text-red-700" title={latest.error ?? undefined}>
          Sync failed {relativeTime(at)}
          {lastOk && <span className="text-gray-500">{` · last good ${relativeTime(lastOk.finished_at ?? lastOk.started_at)}`}</span>}
        </span>
      );
    } else if (latest) {
      fresh = <span title={absoluteTime(at)}>Synced {relativeTime(at)}</span>;
    } else {
      fresh = <span>Never synced</span>;
    }
    body = (
      <div className="min-w-0 space-y-1">
        <div className="flex items-center gap-2" title={`${automated.toLocaleString()} of ${total.toLocaleString()} test cases automated (${Math.round(summary.coverage.pct)}%)`}>
          <ShareBar automated={automated} total={total} />
          <span className="text-sm text-gray-900 tabular-nums whitespace-nowrap">
            {total === 0 ? (
              <span className="text-gray-500">No test cases yet</span>
            ) : (
              <>
                <span className="font-medium">{automated.toLocaleString()}</span>
                <span className="text-gray-500"> of {total.toLocaleString()} automated</span>
              </>
            )}
          </span>
        </div>
        <p className="text-xs text-gray-500 whitespace-nowrap" aria-live="polite">
          {fresh}
        </p>
      </div>
    );
  }

  const noRepo = !!summary && !summary.repo;
  const showSync = canSync && !!summary?.configured;

  return (
    <div className="min-w-0 space-y-1">
      {/* Stacked below 2xl so the row keeps its Actions column on screen at 1280px. */}
      <div className="flex flex-col items-start gap-1.5 2xl:flex-row 2xl:items-center 2xl:justify-between 2xl:gap-3">
        {body}
        <div className="flex items-center gap-1 shrink-0 -ml-1 2xl:ml-0">
          {showSync && (
            <SyncButton
              label={`Sync knowledge base of ${product.name}`}
              busy={sync.busy}
              disabled={noRepo}
              reason={noRepo ? 'Set an automation repo first' : undefined}
              onClick={() => void sync.start()}
            />
          )}
          {openLink}
        </div>
      </div>
      {sync.error && (
        <p role="alert" className="text-xs text-red-700 break-words">
          {sync.error}
        </p>
      )}
      {sync.note && (
        <p role="status" className="text-xs text-gray-600">
          {sync.note}
        </p>
      )}
      {sync.timedOut && <p className="text-xs text-gray-500">Stopped following after 10 minutes; reload to check again.</p>}
    </div>
  );
}
