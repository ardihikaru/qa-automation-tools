import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ExternalLink, GitPullRequest } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import {
  Publication,
  PublicationState,
  EntityType,
  entityHref,
  errorText,
  isAbort,
  isActivePublication,
  listPublications,
  listStories,
  listTestCases,
  publishDrafts,
} from './api';
import { CodeLabel, Disclosure, ErrorNote } from './ui';

const POLL_MS = 4000;

interface Draft {
  type: EntityType;
  code: string;
  epic: string;
  text: string;
}

const STATE_LABEL: Record<PublicationState, string> = {
  queued: 'Publishing…',
  running: 'Publishing…',
  pr_open: 'PR open',
  merged: 'Merged',
  failed: 'Failed',
};
const STATE_TONE: Record<PublicationState, string> = {
  queued: 'bg-blue-50 text-blue-800',
  running: 'bg-blue-50 text-blue-800',
  pr_open: 'bg-violet-100 text-violet-800',
  merged: 'bg-green-100 text-green-800',
  failed: 'bg-red-50 text-red-800',
};

/** State badge of a publication: links to the PR when there is one; the job page is the second link. */
function PubBadge({ p }: { p: Publication }) {
  const cls = `badge ${STATE_TONE[p.state]} gap-1 max-w-full`;
  const label = STATE_LABEL[p.state];
  return (
    <span className="inline-flex items-center gap-2 min-w-0">
      {p.pr_url && (p.state === 'pr_open' || p.state === 'merged') ? (
        <a href={p.pr_url} target="_blank" rel="noreferrer" className={`${cls} hover:underline`} title={p.pr_url}>
          <GitPullRequest className="w-3 h-3 shrink-0" aria-hidden="true" />
          {label}
          <ExternalLink className="w-3 h-3 shrink-0" aria-hidden="true" />
        </a>
      ) : (
        <span className={cls} aria-busy={isActivePublication(p)}>
          {label}
        </span>
      )}
      {p.job_id != null && p.job_id !== '' && (
        <Link to={`/jobs/${encodeURIComponent(String(p.job_id))}`} className="text-xs text-primary-600 hover:underline shrink-0">
          Job #{String(p.job_id)}
        </Link>
      )}
    </span>
  );
}

/**
 * Local drafts per epic with a "Publish drafts" button (one PR per epic on the automation repo)
 * and the state of each publication. Polls the publications while one is queued or running.
 * Shown only to roles that can write; the caller checks.
 */
export default function DraftsPanel({ pid, refreshKey = 0, onChanged }: { pid: number; refreshKey?: number; onChanged?: () => void }) {
  const [drafts, setDrafts] = useState<Draft[] | null>(null);
  const [pubs, setPubs] = useState<Publication[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [rowError, setRowError] = useState<Record<string, string>>({});
  const [publishing, setPublishing] = useState<Set<string>>(new Set());
  const inflight = useRef<Set<string>>(new Set());
  const [pollKey, setPollKey] = useState(0);
  const changedCb = useRef(onChanged);
  changedCb.current = onChanged;
  const hadActive = useRef(false);

  // Local drafts of the product (capped at one page of each kind: 200).
  useEffect(() => {
    const ac = new AbortController();
    Promise.all([listStories(pid, { origin: 'local', page_size: 200 }, ac.signal), listTestCases(pid, { origin: 'local', page_size: 200 }, ac.signal)])
      .then(([s, t]) => {
        setDrafts([
          ...(s.items ?? []).map((x) => ({ type: 'story' as const, code: x.code, epic: x.epic_code, text: x.text })),
          ...(t.items ?? []).map((x) => ({ type: 'test_case' as const, code: x.code, epic: x.epic_code, text: x.scenario })),
        ]);
      })
      .catch((e) => !isAbort(e) && setError(errorText(e, 'Could not load drafts')));
    return () => ac.abort();
  }, [pid, refreshKey]);

  // Publications: load now, then keep polling while any is queued or running.
  useEffect(() => {
    const ac = new AbortController();
    let timer = 0;
    const tick = () => {
      listPublications(pid, ac.signal)
        .then((list) => {
          setPubs(list);
          const active = list.some(isActivePublication);
          if (hadActive.current && !active) changedCb.current?.();
          hadActive.current = active;
          if (active) timer = window.setTimeout(tick, POLL_MS);
        })
        .catch((e) => {
          // A missing endpoint (older server) just means nothing was published.
          if (!isAbort(e)) setError(errorText(e, 'Could not load publications'));
        });
    };
    tick();
    return () => {
      ac.abort();
      window.clearTimeout(timer);
    };
  }, [pid, pollKey]);

  const byEpic = useMemo(() => {
    const m = new Map<string, Draft[]>();
    for (const d of drafts ?? []) m.set(d.epic, [...(m.get(d.epic) ?? []), d]);
    return [...m.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [drafts]);

  // The newest publication of each epic decides the badge of that epic's drafts that are not mapped yet.
  const pubOf = useCallback(
    (d: Draft): Publication | undefined => {
      const mapped = pubs.find((p) => p.codes?.[d.code]);
      if (mapped) return mapped;
      return pubs.find((p) => p.epic_code === d.epic && isActivePublication(p));
    },
    [pubs],
  );

  async function publish(epic: string) {
    if (inflight.current.has(epic)) return;
    inflight.current.add(epic);
    setPublishing((s) => new Set(s).add(epic));
    setRowError((r) => ({ ...r, [epic]: '' }));
    try {
      await publishDrafts(pid, epic);
      setPollKey((n) => n + 1);
    } catch (e) {
      setRowError((r) => ({ ...r, [epic]: errorText(e, 'Could not start publishing') }));
    } finally {
      inflight.current.delete(epic);
      setPublishing((s) => {
        const n = new Set(s);
        n.delete(epic);
        return n;
      });
    }
  }

  if (error && !drafts && pubs.length === 0) return <ErrorNote>{error}</ErrorNote>;
  const recent = pubs.filter((p) => p.state !== 'queued' && p.state !== 'running');
  if (byEpic.length === 0 && pubs.length === 0) return null;

  return (
    <section className="card p-3 min-w-0" aria-label="Local drafts and publishing">
      <Disclosure summary={<>Local drafts{drafts ? ` (${drafts.length})` : ''}{pubs.some(isActivePublication) ? ' · publishing…' : ''}</>}>
        <div className="space-y-4">
          {error && <ErrorNote>{error}</ErrorNote>}
          {byEpic.map(([epic, items]) => {
            const busy = publishing.has(epic) || pubs.some((p) => p.epic_code === epic && isActivePublication(p));
            return (
              <div key={epic} className="min-w-0">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-sm font-medium text-gray-800 min-w-0 truncate" title={epic}>
                    Epic <span className="font-mono">{epic}</span> <span className="text-gray-500">· {items.length} {items.length === 1 ? 'draft' : 'drafts'}</span>
                  </h3>
                  <SubmitButton
                    className="btn btn-secondary text-sm min-h-8"
                    loading={busy}
                    loadingLabel="Publishing…"
                    onClick={() => void publish(epic)}
                    title="Open one pull request with these drafts on the automation repo"
                  >
                    Publish drafts
                  </SubmitButton>
                </div>
                {rowError[epic] && (
                  <p role="alert" className="mt-1 text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2 break-words">
                    {rowError[epic]}
                  </p>
                )}
                <ul className="mt-1 divide-y divide-gray-100">
                  {items.map((d) => {
                    const p = pubOf(d);
                    return (
                      <li key={d.code} className="flex items-center gap-2 py-1.5 min-w-0">
                        <Link to={entityHref(pid, d.type, d.code)} className="shrink-0">
                          <CodeLabel code={d.code} origin="local" removed={false} />
                        </Link>
                        <span className="text-sm text-gray-700 truncate min-w-0 flex-1" title={d.text}>
                          {d.text.replace(/<br\s*\/?>/gi, ' ')}
                        </span>
                        {p && <PubBadge p={p} />}
                      </li>
                    );
                  })}
                </ul>
              </div>
            );
          })}

          {recent.length > 0 && (
            <div>
              <h3 className="text-sm font-medium text-gray-800">Publications</h3>
              <ul className="mt-1 divide-y divide-gray-100">
                {recent.map((p) => (
                  <li key={String(p.id)} className="py-1.5 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-xs text-gray-700">{p.epic_code}</span>
                      <PubBadge p={p} />
                      {p.branch && (
                        <span className="font-mono text-xs text-gray-500 truncate min-w-0" title={p.branch}>
                          {p.branch}
                        </span>
                      )}
                    </div>
                    {p.codes && Object.keys(p.codes).length > 0 && (
                      <ul className="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-gray-600">
                        {Object.entries(p.codes).map(([from, to]) => (
                          <li key={from} className="font-mono break-all">
                            {to} <span className="text-gray-500 font-sans">(was {from})</span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      </Disclosure>
    </section>
  );
}
