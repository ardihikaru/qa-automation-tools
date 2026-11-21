// Knowledge syncs of every product with their duration (kb_syncs).
import { Link } from 'react-router-dom';
import { formatMs, knowledgeHref, shortSha } from '../knowledge/api';
import { Empty, ErrorNote, Pagination, RelTime, SkeletonRows, SyncStatusChip } from '../knowledge/ui';
import { TabProps, listSyncs, productLabel, useHistoryList } from './api';
import { FilterSelect } from './filters';
import { conversationHref } from '../conversations/api';

export default function SyncsTab({ filters, setFilter, refreshKey }: TabProps) {
  const { data, error, loading, retry } = useHistoryList(listSyncs, filters, refreshKey);
  const page = Number(filters.page ?? 1);
  const size = Number(filters.page_size);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <FilterSelect label="Status" value={filters.status ?? ''} options={['running', 'succeeded', 'failed', 'skipped'].map((x) => [x, x])} onChange={(v) => setFilter('status', v)} />
      </div>

      {error && <ErrorNote onRetry={retry}>{error}</ErrorNote>}

      <div className="card overflow-x-auto">
        {loading && !data ? (
          <SkeletonRows rows={6} label="Loading knowledge syncs" />
        ) : !data || data.items.length === 0 ? (
          <Empty>No knowledge syncs match these filters.</Empty>
        ) : (
          <table className={`w-full text-sm ${loading ? 'opacity-60' : ''}`}>
            <thead className="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
              <tr>
                <th className="px-3 py-2">Started</th>
                <th className="px-3 py-2 hidden md:table-cell">Product</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2">Trigger</th>
                <th className="px-3 py-2">Commit</th>
                <th className="px-3 py-2 text-right">Duration</th>
                <th className="px-3 py-2">Error / AI</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {data.items.map((s) => (
                <tr key={s.id}>
                  <td className="px-3 py-2 text-gray-700">
                    <RelTime at={s.started_at} />
                    <div className="text-xs text-gray-500">
                      #{s.id}
                      {/* On a phone the Product column is hidden, so the product folds in here. */}
                      <span className="md:hidden"> · {productLabel(s.product_name, s.product_id)}</span>
                    </div>
                  </td>
                  <td className="px-3 py-2 hidden md:table-cell">
                    <Link to={knowledgeHref(s.product_id, 'syncs')} className="text-primary-700 hover:underline">
                      {s.product_name || `Product ${s.product_id}`}
                    </Link>
                    {s.branch && <div className="text-xs text-gray-500 break-all">{s.branch}</div>}
                  </td>
                  <td className="px-3 py-2">
                    <SyncStatusChip status={s.status} />
                  </td>
                  <td className="px-3 py-2 text-gray-700">{s.trigger}</td>
                  <td className="px-3 py-2 font-mono text-xs text-gray-700">{shortSha(s.commit_sha) || '–'}</td>
                  <td className="px-3 py-2 text-right tabular-nums">{s.duration_ms === null ? <span className="text-gray-500">running</span> : formatMs(s.duration_ms) || '0s'}</td>
                  <td className="px-3 py-2 break-words max-w-md">
                    {s.error && <span className="text-red-700">{s.error}</span>}
                    {s.conversation_id !== null && (
                      <Link to={conversationHref(s.conversation_id)} className="block text-primary-700 hover:underline">
                        AI summary conversation
                      </Link>
                    )}
                  </td>
                </tr>
              ))}
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
          noun="syncs"
          onPage={(p) => setFilter('page', String(p))}
          onPageSize={(n) => setFilter('page_size', String(n))}
        />
      )}
    </div>
  );
}
