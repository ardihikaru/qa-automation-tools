import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Search } from 'lucide-react';
import { Epic, entityHref, listEpics } from './api';
import { useLoad } from './hooks';
import { CoverageBar, MachineMark, Empty, ErrorNote, Pagination, SkeletonRows } from './ui';
import { inputCls } from './styles';
import { Paging, useClampPage, useResetOnChange } from './paging';

interface Props {
  pid: number;
  paging: Paging;
  refreshKey: number;
}

const n = (v: number | undefined) => (v ?? 0).toLocaleString();

function pctOf(e: Epic): number {
  if (e.coverage) return e.coverage.pct;
  const total = e.test_cases ?? 0;
  return total ? ((e.case_counts?.automated ?? 0) * 100) / total : 0;
}

export default function EpicsTab({ pid, paging, refreshKey }: Props) {
  const [query, setQuery] = useState('');
  const { page, size } = paging;
  const { data, loading, error, reload } = useLoad<Epic[]>(`${pid}:epics:${refreshKey}`, (s) => listEpics(pid, false, s));
  useResetOnChange(query, () => paging.setPage(1, true));

  const q = query.trim().toLowerCase();
  const all = (data ?? []).filter((e) => !q || e.code.toLowerCase().includes(q) || (e.title ?? '').toLowerCase().includes(q));
  useClampPage(paging, data ? all.length : null);
  const rows = all.slice((page - 1) * size, page * size);

  return (
    <div className="space-y-3">
      <label className="relative block">
        <Search className="w-4 h-4 text-gray-400 absolute left-2.5 top-1/2 -translate-y-1/2" aria-hidden="true" />
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search epic code or title" aria-label="Search epics" className={`${inputCls} w-full sm:w-80 pl-8`} />
      </label>

      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <SkeletonRows rows={size} label="Loading epics" />}
      {data && all.length === 0 && <Empty>{q ? 'No epics match this search.' : 'No epics yet.'}</Empty>}

      {rows.length > 0 && (
        <div className={`card overflow-x-auto ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
          <table className="w-full min-w-[56rem] table-fixed text-sm">
            <colgroup>
              <col className="w-[10rem]" />
              <col />
              <col className="w-[5rem]" />
              <col className="w-[10rem]" />
              <col className="w-[11rem]" />
              <col className="w-[8rem]" />
            </colgroup>
            <thead className="bg-gray-50 text-left text-gray-600">
              <tr>
                <th className="px-3 py-2 font-medium">Code</th>
                <th className="px-3 py-2 font-medium">Title</th>
                <th className="px-3 py-2 font-medium" title="Test cases">
                  TCs
                </th>
                <th className="px-3 py-2 font-medium">Coverage</th>
                <th className="px-3 py-2 font-medium" title="Verified valid / invalid / total verified">
                  Verified ✓ / ✗ / total
                </th>
                <th className="px-3 py-2 font-medium" title="Latest run per test case">
                  Last run ✓ / ✗
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {rows.map((e) => {
                const valid = e.verified_valid ?? 0;
                const invalid = e.verified_invalid ?? 0;
                return (
                  <tr key={e.code} className="kb-row">
                    <td className="px-3 py-2">
                      <Link to={entityHref(pid, 'epic', e.code)} className="font-mono text-xs text-primary-700 hover:underline break-all min-h-8 inline-flex items-center">
                        {e.code}
                      </Link>
                    </td>
                    <td className="px-3 py-2 truncate" title={e.title}>
                      {e.title} <MachineMark i18n={e.i18n} />
                    </td>
                    <td className="px-3 py-2 tabular-nums">{n(e.test_cases)}</td>
                    <td className="px-3 py-2">
                      <CoverageBar value={pctOf(e)} />
                    </td>
                    <td className="px-3 py-2 tabular-nums text-xs">
                      <span className="text-green-700">{n(valid)}</span> / <span className="text-red-700">{n(invalid)}</span> / {(valid + invalid).toLocaleString()}
                    </td>
                    <td className="px-3 py-2 tabular-nums text-xs">
                      <span className="text-green-700">{n(e.last_pass)}</span> / <span className="text-red-700">{n(e.last_fail)}</span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {data && all.length > 0 && <Pagination page={page} pageSize={size} total={all.length} onPage={paging.setPage} onPageSize={paging.setSize} noun="Epics" disabled={loading} />}
    </div>
  );
}
