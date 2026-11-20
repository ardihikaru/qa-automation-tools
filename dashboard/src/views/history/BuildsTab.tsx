// Builds: case_create / case_run jobs with machine time apart from time waiting for approval.
import { Fragment, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight } from 'lucide-react';
import { absoluteTime, entityHref, formatMs } from '../knowledge/api';
import { JOB_CHIP_CLASS, jobChipStatus, jobHref } from '../knowledge/CaseAutomation';
import { Empty, ErrorNote, Pagination, RelTime, SkeletonRows } from '../knowledge/ui';
import { Build, StepMs, STEP_COLOR, TabProps, WAITING_STEPS, listBuilds, plural, useHistoryList } from './api';
import { FilterSelect, FilterText } from './filters';

const TYPE_LABEL: Record<string, string> = { case_create: 'Build', case_run: 'Run case' };

/** A stacked bar of a build's steps; approval (a human's time) is amber. */
export function StepBar({ steps }: { steps: StepMs[] }) {
  const total = steps.reduce((s, x) => s + x.ms, 0);
  if (total <= 0) return <span className="text-xs text-gray-400">no events</span>;
  const label = steps.map((s) => `${s.step} ${formatMs(s.ms)}`).join(', ');
  return (
    <div className="flex h-2.5 w-40 overflow-hidden rounded-full bg-gray-100" role="img" aria-label={`Steps: ${label}`} title={label}>
      {steps.map((s) => (
        <span key={s.step} className={`${STEP_COLOR[s.step] ?? 'bg-gray-400'} h-full`} style={{ width: `${Math.max(1, (s.ms / total) * 100)}%` }} />
      ))}
    </div>
  );
}

function StepList({ b }: { b: Build }) {
  return (
    <div className="space-y-2 text-sm">
      {b.steps.length === 0 ? (
        <p className="text-gray-500">No events were recorded for this job.</p>
      ) : (
        <ul className="flex flex-wrap gap-x-5 gap-y-1">
          {b.steps.map((s) => (
            <li key={s.step} className="inline-flex items-center gap-1.5">
              <span className={`inline-block w-2.5 h-2.5 rounded-sm ${STEP_COLOR[s.step] ?? 'bg-gray-400'}`} aria-hidden="true" />
              <span className="font-mono text-gray-700">{s.step}</span>
              <span className="tabular-nums text-gray-900">{formatMs(s.ms)}</span>
              {WAITING_STEPS.has(s.step) && <span className="text-xs text-amber-800">(waiting for a human)</span>}
            </li>
          ))}
        </ul>
      )}
      <p className="text-xs text-gray-500">
        Created {absoluteTime(b.created_at)}
        {b.started_at && <> · started {absoluteTime(b.started_at)}</>}
        {b.completed_at && <> · finished {absoluteTime(b.completed_at)}</>}
        {b.approvals > 0 && <> · {b.approvals} approval round{b.approvals === 1 ? '' : 's'}</>}
        {b.runs > 0 && <> · {b.runs} Playwright run{b.runs === 1 ? '' : 's'} logged</>}
        {b.batch_id && <> · epic batch {b.batch_id}</>}. Step times run from one event to the next and include any backend restart.
      </p>
      {b.error && <p className="text-sm text-red-700 break-words">{b.error}</p>}
      {b.pr_url && /^https?:\/\//i.test(b.pr_url) && (
        <a href={b.pr_url} target="_blank" rel="noopener noreferrer" className="text-sm text-primary-700 hover:underline">
          Pull request<span className="sr-only"> (opens in a new tab)</span>
        </a>
      )}
    </div>
  );
}

const ms = (v: number | null | undefined, running: boolean) => {
  if (v === null || v === undefined) return <span className="text-gray-400">–</span>;
  return (
    <span className="tabular-nums whitespace-nowrap">
      {formatMs(v) || '0s'}
      {running && <span className="text-xs text-gray-500"> so far</span>}
    </span>
  );
};

export default function BuildsTab({ filters, setFilter, refreshKey }: TabProps) {
  const { data, error, loading, retry } = useHistoryList(listBuilds, filters, refreshKey);
  const [open, setOpen] = useState<string | null>(null);
  const page = Number(filters.page ?? 1);
  const size = Number(filters.page_size);
  const s = data?.summary;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <FilterSelect label="Type" value={filters.type ?? ''} options={[['case_create', 'Build (create automation)'], ['case_run', 'Run case']]} onChange={(v) => setFilter('type', v)} />
        <FilterSelect label="Status" value={filters.status ?? ''} options={['queued', 'running', 'completed', 'failed'].map((x) => [x, x])} onChange={(v) => setFilter('status', v)} />
        <FilterText label="Case" value={filters.case ?? ''} placeholder="TC-AUTH-001" onChange={(v) => setFilter('case', v)} />
        <FilterText label="Epic" value={filters.epic ?? ''} placeholder="AUTH" onChange={(v) => setFilter('epic', v)} />
      </div>

      {s && (
        <p className="text-sm text-gray-700" aria-live="polite">
          <strong className="tabular-nums">{plural(s.count, 'job')}</strong> · {s.completed} completed · {s.failed} failed · {s.running} queued or running. Finished jobs average{' '}
          <strong>{formatMs(s.avg_machine_ms) || '–'}</strong> machine time and <strong className="text-amber-800">{formatMs(s.avg_waiting_ms) || '–'}</strong> waiting for approval.
        </p>
      )}

      {error && <ErrorNote onRetry={retry}>{error}</ErrorNote>}

      <div className="card overflow-x-auto">
        {loading && !data ? (
          <SkeletonRows rows={6} label="Loading builds" />
        ) : !data || data.items.length === 0 ? (
          <Empty>No builds match these filters.</Empty>
        ) : (
          <table className={`w-full text-sm ${loading ? 'opacity-60' : ''}`}>
            <thead className="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
              <tr>
                <th className="px-3 py-2 w-8"><span className="sr-only">Details</span></th>
                <th className="px-3 py-2">Job</th>
                <th className="px-3 py-2">Case</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Created</th>
                <th className="px-3 py-2 text-right">Queue</th>
                <th className="px-3 py-2 text-right">Machine</th>
                <th className="px-3 py-2 text-right">Waiting for approval</th>
                <th className="px-3 py-2 text-right">Total</th>
                <th className="px-3 py-2">Steps</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {data.items.map((b) => {
                const st = jobChipStatus(b.status, b.outcome);
                const running = !b.completed_at && !!b.started_at;
                const isOpen = open === b.id;
                return (
                  <Fragment key={b.id}>
                    <tr className="align-middle">
                      <td className="px-3 py-2">
                        <button
                          type="button"
                          aria-expanded={isOpen}
                          aria-label={`${isOpen ? 'Hide' : 'Show'} the steps of ${b.id}`}
                          onClick={() => setOpen(isOpen ? null : b.id)}
                          className="inline-flex items-center justify-center w-8 h-8 rounded hover:bg-gray-100"
                        >
                          <ChevronRight className={`w-4 h-4 text-gray-500 transition-transform ${isOpen ? 'rotate-90' : ''}`} aria-hidden="true" />
                        </button>
                      </td>
                      <td className="px-3 py-2 whitespace-nowrap">
                        <Link to={jobHref(b.id)} state={{ pid: b.product_id, code: b.case_code }} className="font-mono text-primary-700 hover:underline">
                          {b.id}
                        </Link>
                        <div className="text-xs text-gray-500">
                          {TYPE_LABEL[b.type] ?? b.type}
                          {b.product_name && <> · {b.product_name}</>}
                        </div>
                      </td>
                      <td className="px-3 py-2 whitespace-nowrap">
                        {b.case_code && b.product_id > 0 ? (
                          <Link to={entityHref(b.product_id, 'test_case', b.case_code)} className="font-mono text-primary-700 hover:underline">
                            {b.case_code}
                          </Link>
                        ) : (
                          <span className="font-mono">{b.case_code || '–'}</span>
                        )}
                        {b.epic_code && <div className="text-xs text-gray-500">Epic {b.epic_code}</div>}
                      </td>
                      <td className="px-3 py-2">
                        <span className={`badge whitespace-nowrap ${JOB_CHIP_CLASS[st] ?? 'bg-gray-100 text-gray-700'}`}>{st}</span>
                      </td>
                      <td className="px-3 py-2 text-gray-600">
                        <RelTime at={b.created_at} />
                      </td>
                      <td className="px-3 py-2 text-right">{ms(b.queue_ms, false)}</td>
                      <td className="px-3 py-2 text-right font-medium">{ms(b.machine_ms, running)}</td>
                      <td className="px-3 py-2 text-right text-amber-800">{b.started_at ? ms(b.waiting_ms, running && b.waiting_ms > 0) : ms(null, false)}</td>
                      <td className="px-3 py-2 text-right">{ms(b.active_ms, running)}</td>
                      <td className="px-3 py-2">
                        <StepBar steps={b.steps} />
                      </td>
                    </tr>
                    {isOpen && (
                      <tr className="bg-gray-50/60">
                        <td />
                        <td colSpan={9} className="px-3 py-3">
                          <StepList b={b} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {data && data.total > 0 && (
        <Pagination
          page={page}
          pageSize={size}
          total={data.total}
          disabled={loading}
          noun="builds"
          onPage={(p) => setFilter('page', String(p))}
          onPageSize={(n) => setFilter('page_size', String(n))}
        />
      )}
    </div>
  );
}
