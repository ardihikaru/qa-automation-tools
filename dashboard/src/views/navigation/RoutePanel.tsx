// Drill-down of one route: features, cases with their latest result and job, specs and the runs
// that visited it (NAVIGATION_MAP.md section 7).
import { Link } from 'react-router-dom';
import { ExternalLink, X } from 'lucide-react';
import { entityHref } from '../knowledge/api';
import { useLoad } from '../knowledge/hooks';
import { CaseStatusChip, Empty, ErrorNote, ReleaseChip, RelTime, SkeletonRows } from '../knowledge/ui';
import { NavCase, NavRouteDetail, STATUS_CHIP, STATUS_LABEL, getNavRoute } from './api';

const RESULT_CHIP: Record<string, string> = {
  pass: 'bg-green-100 text-green-800',
  fail: 'bg-red-100 text-red-800',
  skipped: 'bg-gray-100 text-gray-700',
};

const SOURCE_LABEL: Record<string, string> = {
  scenario: 'scenario',
  spec: 'spec',
  observed: 'run',
  wildcard: 'under this route',
};

function Section({ title, count, children }: { title: string; count?: number; children: React.ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
        {title}
        {count !== undefined && <span className="ml-1.5 font-normal normal-case tracking-normal text-gray-400 tabular-nums">{count}</span>}
      </h3>
      {children}
    </section>
  );
}

function duration(ms: number | null): string {
  if (ms === null || ms === undefined) return '';
  if (ms < 1000) return `${ms} ms`;
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s} s` : `${Math.floor(s / 60)} min ${s % 60} s`;
}

function CaseRow({ pid, c }: { pid: number; c: NavCase }) {
  return (
    <li className="py-2 space-y-1">
      <div className="flex flex-wrap items-center gap-1.5">
        <Link to={entityHref(pid, 'test_case', c.code)} className="font-mono text-xs text-primary-700 hover:underline">
          {c.code}
        </Link>
        <CaseStatusChip status={c.status} />
        {c.priority && <span className="text-xs text-gray-500">{c.priority}</span>}
        {c.result && <span className={`badge ${RESULT_CHIP[c.result] ?? ''}`}>{c.result}</span>}
        {c.run_at && (
          <span className="text-xs text-gray-500">
            <RelTime at={c.run_at} />
          </span>
        )}
      </div>
      {c.scenario && <p className="text-xs text-gray-600 line-clamp-2 break-words">{c.scenario}</p>}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500">
        <span>via {c.link_sources.map((s) => SOURCE_LABEL[s] ?? s).join(', ')}</span>
        {c.last_job && (
          <Link to={`/jobs/${encodeURIComponent(c.last_job.id)}`} className="inline-flex items-center gap-1 text-primary-700 hover:underline">
            last {c.last_job.type === 'case_create' ? 'automation' : 'run'}: {c.last_job.status}
            {c.last_job.duration_ms !== null && ` · ${duration(c.last_job.duration_ms)}`}
          </Link>
        )}
        {c.last_run && (
          <Link to={`/history?tab=runs&product=${pid}&case=${encodeURIComponent(c.code)}`} className="text-primary-700 hover:underline" title="Run history of this case">
            last Playwright run: {c.last_run.result}, {duration(c.last_run.duration_ms)}
          </Link>
        )}
      </div>
    </li>
  );
}

export default function RoutePanel({ pid, routeKey, onClose }: { pid: number; routeKey: string; onClose: () => void }) {
  const { data, loading, error, reload } = useLoad<NavRouteDetail>(`${pid}:${routeKey}`, (s) => getNavRoute(pid, routeKey, s));

  return (
    <aside className="card" aria-label="Route details">
      <div className="card-header flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-xs text-gray-500">{data ? `${data.surface === 'web' ? 'Page' : 'API route'} · ${data.app}` : 'Route'}</p>
          <h2 className="font-mono text-sm font-semibold text-gray-900 break-all">{data?.path ?? '…'}</h2>
        </div>
        <button type="button" onClick={onClose} className="btn btn-secondary !p-1.5 shrink-0" aria-label="Close route details">
          <X className="w-4 h-4" aria-hidden="true" />
        </button>
      </div>
      <div className="card-body space-y-5">
        {loading && !data && <SkeletonRows rows={6} label="Loading route" />}
        {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
        {data && (
          <>
            <div className="flex flex-wrap items-center gap-1.5">
              <span className={`badge ${STATUS_CHIP[data.status]}`}>{STATUS_LABEL[data.status]}</span>
              {data.methods.map((m) => (
                <span key={m} className="badge bg-gray-100 text-gray-700 font-mono">
                  {m}
                </span>
              ))}
              {data.sources.map((s) => (
                <span key={s} className="badge bg-white text-gray-500 ring-1 ring-inset ring-gray-200" title="Where this route was found">
                  {s === 'repo' ? 'automation repo' : s}
                </span>
              ))}
            </div>
            {data.page_objects.length > 0 && (
              <p className="text-xs text-gray-600">
                Page object: <span className="font-mono">{data.page_objects.join(', ')}</span>
              </p>
            )}

            <Section title="Features" count={data.features.length}>
              {data.features.length === 0 ? (
                <p className="text-xs text-gray-500">No feature of the module catalogue names this route.</p>
              ) : (
                <ul className="space-y-1.5">
                  {data.features.map((f) => (
                    <li key={f.code} className="flex items-start gap-2 text-sm">
                      <Link to={entityHref(pid, 'feature', f.code)} className="font-mono text-xs text-primary-700 hover:underline shrink-0 pt-0.5">
                        {f.code}
                      </Link>
                      <span className="text-gray-700 min-w-0 break-words flex-1">{f.name}</span>
                      <ReleaseChip status={f.release_status} />
                    </li>
                  ))}
                </ul>
              )}
            </Section>

            <Section title="Test cases" count={data.cases.length}>
              {data.cases.length === 0 ? (
                <Empty>No test case calls or opens this route yet.</Empty>
              ) : (
                <ul className="divide-y divide-gray-100">
                  {data.cases.map((c) => (
                    <CaseRow key={c.code} pid={pid} c={c} />
                  ))}
                </ul>
              )}
            </Section>

            {data.related_total > 0 && (
              <Section title="Related through a feature" count={data.related_total}>
                <p className="text-xs text-gray-500">Cases of the stories of this route's features. They do not colour the route.</p>
                <div className="flex flex-wrap gap-1.5">
                  {data.related_cases.map((c) => (
                    <Link key={c.code} to={entityHref(pid, 'test_case', c.code)} className="badge bg-gray-50 text-gray-700 ring-1 ring-inset ring-gray-200 font-mono hover:bg-gray-100" title={c.status}>
                      {c.code}
                    </Link>
                  ))}
                  {data.related_total > data.related_cases.length && <span className="text-xs text-gray-500">+{data.related_total - data.related_cases.length} more</span>}
                </div>
              </Section>
            )}

            <Section title="Specs" count={data.specs.length}>
              {data.specs.length === 0 ? (
                <p className="text-xs text-gray-500">No spec of the automation repo uses this route.</p>
              ) : (
                <ul className="space-y-1">
                  {data.specs.map((s) => (
                    <li key={s} className="font-mono text-xs text-gray-700 break-all">
                      {s}
                    </li>
                  ))}
                </ul>
              )}
            </Section>

            <Section title="Seen in runs" count={data.observations.length}>
              {data.observations.length === 0 ? (
                <p className="text-xs text-gray-500">No executor run has visited this route yet.</p>
              ) : (
                <ul className="space-y-1">
                  {data.observations.map((o) => (
                    <li key={`${o.job_id}:${o.case_code}:${o.method}`} className="flex flex-wrap items-center gap-x-2 text-xs text-gray-600">
                      <Link to={`/jobs/${encodeURIComponent(o.job_id)}`} className="inline-flex items-center gap-1 text-primary-700 hover:underline">
                        <ExternalLink className="w-3 h-3" aria-hidden="true" />
                        {o.case_code}
                      </Link>
                      {o.method && <span className="font-mono">{o.method}</span>}
                      <span className="tabular-nums">{o.hits}×</span>
                      <RelTime at={o.observed_at} />
                    </li>
                  ))}
                </ul>
              )}
            </Section>
          </>
        )}
      </div>
    </aside>
  );
}
