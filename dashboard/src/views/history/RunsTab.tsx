// Playwright runs: every report the QA tools received, with its duration and per-case durations.
import { Fragment, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight } from 'lucide-react';
import { absoluteTime, entityHref, errorText, formatMs, isAbort } from '../knowledge/api';
import { jobHref } from '../knowledge/CaseAutomation';
import { Empty, ErrorNote, Loading, Pagination, RelTime, SkeletonRows } from '../knowledge/ui';
import { OUTCOME_CLASS, RunDetail, SOURCE_LABEL, TabProps, getRun, listRuns, plural, useHistoryList } from './api';
import { FilterSelect, FilterText } from './filters';

function Outcome({ value }: { value: string }) {
  return <span className={`badge whitespace-nowrap ${OUTCOME_CLASS[value] ?? 'bg-gray-100 text-gray-700'}`}>{value}</span>;
}

function RunCases({ id }: { id: number }) {
  const [run, setRun] = useState<RunDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [sort, setSort] = useState<'duration' | 'code'>('duration');
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const ac = new AbortController();
    setError(null);
    getRun(id, ac.signal)
      .then(setRun)
      .catch((e) => {
        if (!isAbort(e)) setError(errorText(e, 'Could not load the run'));
      });
    return () => ac.abort();
  }, [id, retry]);
  const rows = useMemo(() => {
    const list = [...(run?.case_rows ?? [])];
    if (sort === 'code') list.sort((a, b) => a.case_code.localeCompare(b.case_code));
    return list;
  }, [run, sort]);
  if (error) return <ErrorNote onRetry={() => setRetry((n) => n + 1)}>{error}</ErrorNote>;
  if (!run) return <Loading label="Loading the cases of this run…" />;
  if (rows.length === 0) return <p className="text-sm text-gray-500">This report has no test titled with a case code.</p>;
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs text-gray-600">
        Sort by
        {(['duration', 'code'] as const).map((k) => (
          <button key={k} type="button" aria-pressed={sort === k} onClick={() => setSort(k)} className={`rounded px-2 py-1 ${sort === k ? 'bg-primary-100 text-primary-800' : 'hover:bg-gray-100'}`}>
            {k === 'duration' ? 'slowest first' : 'case code'}
          </button>
        ))}
      </div>
      <div className="max-h-96 overflow-auto rounded border border-gray-200 bg-white">
        <table className="w-full text-sm">
          <thead className="sticky top-0 bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
            <tr>
              <th className="px-3 py-1.5">Case</th>
              <th className="px-3 py-1.5">Result</th>
              <th className="px-3 py-1.5 text-right">Duration</th>
              <th className="px-3 py-1.5 text-right">Tests</th>
              <th className="px-3 py-1.5 text-right">Attempts</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {rows.map((c) => (
              <tr key={c.case_code}>
                <td className="px-3 py-1.5 whitespace-nowrap">
                  {c.known ? (
                    <Link to={entityHref(run.product_id, 'test_case', c.case_code)} className="font-mono text-primary-700 hover:underline">
                      {c.case_code}
                    </Link>
                  ) : (
                    <span className="font-mono text-gray-600" title="Not an active case of this product">
                      {c.case_code}
                    </span>
                  )}
                  {c.epic_code && <span className="ml-2 text-xs text-gray-500">{c.epic_code}</span>}
                </td>
                <td className="px-3 py-1.5">
                  <Outcome value={c.result} />
                </td>
                <td className="px-3 py-1.5 text-right tabular-nums">{formatMs(c.duration_ms) || '0s'}</td>
                <td className="px-3 py-1.5 text-right tabular-nums">{c.tests}</td>
                <td className="px-3 py-1.5 text-right tabular-nums">{c.attempts}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default function RunsTab({ filters, setFilter, refreshKey }: TabProps) {
  const { data, error, loading, retry } = useHistoryList(listRuns, filters, refreshKey);
  const [open, setOpen] = useState<number | null>(null);
  const page = Number(filters.page ?? 1);
  const size = Number(filters.page_size);
  const s = data?.summary;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <FilterSelect label="Source" value={filters.source ?? ''} options={Object.entries(SOURCE_LABEL)} onChange={(v) => setFilter('source', v)} />
        <FilterSelect label="Outcome" value={filters.outcome ?? ''} options={['pass', 'fail', 'skipped'].map((x) => [x, x])} onChange={(v) => setFilter('outcome', v)} />
        <FilterText label="Case" value={filters.case ?? ''} placeholder="TC-AUTH-001" onChange={(v) => setFilter('case', v)} />
        <FilterText label="Epic" value={filters.epic ?? ''} placeholder="AUTH" onChange={(v) => setFilter('epic', v)} />
        <FilterText label="Job" value={filters.job ?? ''} placeholder="job-…" onChange={(v) => setFilter('job', v)} />
      </div>

      {s && (
        <p className="text-sm text-gray-700" aria-live="polite">
          <strong className="tabular-nums">{plural(s.count, 'run')}</strong>, by outcome: {s.pass} passed, {s.fail} failed, {s.skipped} skipped. Average duration <strong>{formatMs(s.avg_duration_ms) || '–'}</strong>.{' '}
          <span className="text-gray-500">
            {data?.since ? `Recorded since ${absoluteTime(data.since)}.` : 'Nothing recorded yet.'} A run is logged when a report reaches the QA tools; an executor run that failed or returned no report is
            not a run here (see its build).
          </span>
        </p>
      )}

      {error && <ErrorNote onRetry={retry}>{error}</ErrorNote>}

      <div className="card overflow-x-auto">
        {loading && !data ? (
          <SkeletonRows rows={6} label="Loading Playwright runs" />
        ) : !data || data.items.length === 0 ? (
          <Empty>No Playwright runs match these filters.</Empty>
        ) : (
          <table className={`w-full text-sm ${loading ? 'opacity-60' : ''}`}>
            <thead className="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
              <tr>
                <th className="px-3 py-2 w-8"><span className="sr-only">Cases</span></th>
                <th className="px-3 py-2">Started</th>
                <th className="px-3 py-2">Source</th>
                <th className="px-3 py-2">Ref / job</th>
                <th className="px-3 py-2">Case</th>
                <th className="px-3 py-2 text-right">Duration</th>
                <th className="px-3 py-2 text-right">Passed</th>
                <th className="px-3 py-2 text-right">Failed</th>
                <th className="px-3 py-2 text-right">Flaky</th>
                <th className="px-3 py-2 text-right">Skipped</th>
                <th className="px-3 py-2 text-right">Cases</th>
                <th className="px-3 py-2">Outcome</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {data.items.map((r) => {
                const isOpen = open === r.id;
                return (
                  <Fragment key={r.id}>
                    <tr>
                      <td className="px-3 py-2">
                        <button
                          type="button"
                          aria-expanded={isOpen}
                          aria-label={`${isOpen ? 'Hide' : 'Show'} the cases of run ${r.id}`}
                          onClick={() => setOpen(isOpen ? null : r.id)}
                          className="inline-flex items-center justify-center w-8 h-8 rounded hover:bg-gray-100"
                        >
                          <ChevronRight className={`w-4 h-4 text-gray-500 transition-transform ${isOpen ? 'rotate-90' : ''}`} aria-hidden="true" />
                        </button>
                      </td>
                      <td className="px-3 py-2 text-gray-700">
                        <RelTime at={r.started_at} />
                        <div className="text-xs text-gray-500">{r.product_name}</div>
                      </td>
                      <td className="px-3 py-2 whitespace-nowrap">
                        {SOURCE_LABEL[r.source] ?? r.source}
                        {r.run_index !== null && <div className="text-xs text-gray-500">executor run #{r.run_index + 1}</div>}
                      </td>
                      <td className="px-3 py-2 max-w-[16rem]">
                        {r.job_id ? (
                          <Link to={jobHref(r.job_id)} state={{ pid: r.product_id, code: r.case_code }} className="font-mono text-primary-700 hover:underline break-all">
                            {r.job_id}
                          </Link>
                        ) : (
                          <span className="font-mono text-gray-700 break-all">{r.run_ref}</span>
                        )}
                      </td>
                      <td className="px-3 py-2 whitespace-nowrap">
                        {r.case_code ? (
                          <Link to={entityHref(r.product_id, 'test_case', r.case_code)} className="font-mono text-primary-700 hover:underline">
                            {r.case_code}
                          </Link>
                        ) : (
                          <span className="text-gray-400">–</span>
                        )}
                        {r.case_result && (
                          <div className="text-xs text-gray-500">
                            {filters.case}: {r.case_result}
                            {r.case_duration_ms !== null && r.case_duration_ms !== undefined && <> in {formatMs(r.case_duration_ms) || '0s'}</>}
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums font-medium">{r.duration_ms === null ? '–' : formatMs(r.duration_ms) || '0s'}</td>
                      <td className="px-3 py-2 text-right tabular-nums text-green-700">{r.passed}</td>
                      <td className="px-3 py-2 text-right tabular-nums text-red-700">{r.failed}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{r.flaky}</td>
                      <td className="px-3 py-2 text-right tabular-nums text-gray-600">{r.skipped}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{r.cases}</td>
                      <td className="px-3 py-2">
                        <Outcome value={r.outcome} />
                      </td>
                    </tr>
                    {isOpen && (
                      <tr className="bg-gray-50/60">
                        <td />
                        <td colSpan={11} className="px-3 py-3">
                          <RunCases id={r.id} />
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
          noun="runs"
          onPage={(p) => setFilter('page', String(p))}
          onPageSize={(n) => setFilter('page_size', String(n))}
        />
      )}
    </div>
  );
}
