// A page of stored conversations (live or archived) with open and archive / unarchive actions.
import { Link } from 'react-router-dom';
import { RelTime, Empty, ErrorNote, Pagination, SkeletonRows } from '../knowledge/ui';
import ArchiveButton from './ArchiveButton';
import { ConversationPage, KIND_LABEL, conversationHref, formatBytes, ownerHref } from './api';

export default function ConversationList({
  data,
  error,
  loading,
  retry,
  page,
  pageSize,
  onPage,
  onPageSize,
  onChanged,
  canAct,
  archived,
}: {
  data: ConversationPage | null;
  error: string | null;
  loading: boolean;
  retry: () => void;
  page: number;
  pageSize: number;
  onPage: (p: number) => void;
  onPageSize: (s: number) => void;
  onChanged: () => void;
  canAct: boolean;
  archived: boolean;
}) {
  return (
    <div className="space-y-3">
      {error && <ErrorNote onRetry={retry}>{error}</ErrorNote>}
      {/* relative: the sr-only header text is absolute and must stay inside the scroller, or it widens the page on a phone. */}
      <div className="card overflow-x-auto relative">
        {loading && !data ? (
          <SkeletonRows rows={5} label="Loading conversations" />
        ) : !data || data.items.length === 0 ? (
          <Empty>{archived ? 'Nothing is archived.' : 'No AI conversations match these filters.'}</Empty>
        ) : (
          <table className={`w-full text-sm ${loading ? 'opacity-60' : ''}`}>
            <thead className="bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
              <tr>
                <th className="px-3 py-2">Conversation</th>
                <th className="px-3 py-2 hidden md:table-cell">Product</th>
                <th className="px-3 py-2">Kind</th>
                <th className="px-3 py-2">Work</th>
                <th className="px-3 py-2 text-right">Messages</th>
                <th className="px-3 py-2 text-right">Size</th>
                <th className="px-3 py-2">{archived ? 'Archived' : 'Updated'}</th>
                <th className="px-3 py-2">
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {data.items.map((c) => {
                const owner = ownerHref(c);
                const product = c.product_name || (c.product_id ? `Product ${c.product_id}` : '');
                return (
                  <tr key={c.id}>
                    <td className="px-3 py-2 min-w-0">
                      {canAct ? (
                        <Link to={conversationHref(c.id)} className="font-medium text-primary-700 hover:underline break-all">
                          {c.title || c.ref}
                        </Link>
                      ) : (
                        <span className="font-medium break-all">{c.title || c.ref}</span>
                      )}
                      {(product || c.feature) && (
                        <div className="text-xs text-gray-500">
                          {/* On a phone the Product column is hidden, so the product folds in here. */}
                          {product && <span className="md:hidden">{product}</span>}
                          {product && c.feature && <span className="md:hidden"> · </span>}
                          {c.feature}
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-2 hidden md:table-cell text-gray-700">{product || <span className="text-gray-400">–</span>}</td>
                    <td className="px-3 py-2 whitespace-nowrap">{KIND_LABEL[c.kind] ?? c.kind}</td>
                    <td className="px-3 py-2 max-w-[16rem]">
                      {owner ? (
                        <Link to={owner} className="font-mono text-xs text-primary-700 hover:underline break-all">
                          {c.ref}
                        </Link>
                      ) : (
                        <span className="font-mono text-xs text-gray-600 break-all">{c.ref}</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-right tabular-nums">{c.message_count}</td>
                    <td className="px-3 py-2 text-right tabular-nums whitespace-nowrap">{formatBytes(c.bytes)}</td>
                    <td className="px-3 py-2 text-gray-600">
                      <RelTime at={archived ? c.archived_at : c.updated_at} />
                      {archived && c.archived_by_name && <div className="text-xs text-gray-500">by {c.archived_by_name}</div>}
                    </td>
                    <td className="px-3 py-2 text-right">{canAct && <ArchiveButton conversation={c} onDone={onChanged} compact />}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
      {data && data.total > 0 && <Pagination page={page} pageSize={pageSize} total={data.total} disabled={loading} noun="conversations" onPage={onPage} onPageSize={onPageSize} />}
    </div>
  );
}
