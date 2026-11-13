import { useEffect, useState } from 'react';
import { Search, StickyNote } from 'lucide-react';
import { Epic, Page, Story, listStories } from './api';
import { useDebounced, useLoad } from './hooks';
import { CodeLabel, Empty, ErrorNote, Loading, Pagination, RemovedBadge } from './ui';
import { inputCls, selectCls } from './styles';
import { Target } from './EntityPanel';

const PAGE_SIZE = 50;

interface Props {
  pid: number;
  epics: Epic[];
  features: { code: string; name: string }[];
  refreshKey: number;
  selected: string | null;
  onOpen: (t: Target) => void;
}

export default function StoriesTab({ pid, epics, features, refreshKey, selected, onOpen }: Props) {
  const [query, setQuery] = useState('');
  const [epic, setEpic] = useState('');
  const [feature, setFeature] = useState('');
  const [tag, setTag] = useState('');
  const [origin, setOrigin] = useState('');
  const [includeRemoved, setIncludeRemoved] = useState(false);
  const [page, setPage] = useState(1);
  const q = useDebounced(query.trim(), 300);
  const f = useDebounced(feature.trim(), 300);
  const t = useDebounced(tag.trim(), 300);

  useEffect(() => setPage(1), [q, epic, f, t, origin, includeRemoved]);

  const filter = { q, epic, feature: f, tag: t, origin, include_removed: includeRemoved, page, page_size: PAGE_SIZE };
  const { data, loading, error, reload } = useLoad<Page<Story>>(`${pid}:${JSON.stringify(filter)}:${refreshKey}`, (s) => listStories(pid, filter, s));
  const filtered = !!(q || epic || f || t || origin);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <label className="relative flex-1 min-w-[14rem]">
          <Search className="w-4 h-4 text-gray-400 absolute left-2.5 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search code or text" aria-label="Search stories" className={`${inputCls} w-full pl-8`} />
        </label>
        <select value={epic} onChange={(e) => setEpic(e.target.value)} aria-label="Epic" className={`${selectCls} max-w-[16rem]`}>
          <option value="">All epics</option>
          {epics.map((e) => (
            <option key={e.code} value={e.code}>
              {e.code}
              {e.title ? ` — ${e.title.length > 40 ? `${e.title.slice(0, 40)}…` : e.title}` : ''}
            </option>
          ))}
        </select>
        <input value={feature} onChange={(e) => setFeature(e.target.value)} list="kb-stories-features" placeholder="Feature code" aria-label="Feature" className={`${inputCls} w-36 font-mono`} />
        <datalist id="kb-stories-features">
          {features.map((x) => (
            <option key={x.code} value={x.code}>
              {x.name}
            </option>
          ))}
        </datalist>
        <input value={tag} onChange={(e) => setTag(e.target.value)} placeholder="Tag" aria-label="Tag" className={`${inputCls} w-32`} />
        <select value={origin} onChange={(e) => setOrigin(e.target.value)} aria-label="Origin" className={selectCls}>
          <option value="">Synced + local</option>
          <option value="sync">Synced</option>
          <option value="local">Local drafts</option>
        </select>
        <label className="inline-flex items-center gap-2 text-sm text-gray-700 min-h-8">
          <input type="checkbox" checked={includeRemoved} onChange={(e) => setIncludeRemoved(e.target.checked)} />
          Include removed
        </label>
      </div>

      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <Loading />}
      {data && data.items.length === 0 && <Empty>{filtered ? 'No stories match these filters.' : 'No user stories yet.'}</Empty>}

      {data && data.items.length > 0 && (
        <div className={`card overflow-hidden ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
          <ul className="divide-y divide-gray-100">
            {data.items.map((s) => {
              const removed = !!s.removed_at;
              return (
                <li key={s.code}>
                  <button
                    type="button"
                    onClick={() => onOpen({ type: 'story', code: s.code })}
                    className={`kb-row w-full text-left grid grid-cols-[minmax(0,1fr)] md:grid-cols-[12rem_minmax(0,1fr)_9rem] gap-x-4 gap-y-1 px-4 py-2.5 min-h-8 ${selected === s.code ? 'bg-primary-50' : ''}`}
                  >
                    <span className="flex items-center gap-2 min-w-0">
                      <CodeLabel code={s.code} origin={s.origin} removed={removed} />
                      {s.annotation && <StickyNote className="w-3.5 h-3.5 text-amber-500 shrink-0" aria-label="has a note" />}
                    </span>
                    <span className={`text-sm line-clamp-2 break-words ${removed ? 'line-through text-gray-400' : 'text-gray-800'}`} title={s.text}>
                      {s.text}
                    </span>
                    <span className="flex flex-wrap items-center gap-1.5 md:justify-end">
                      <span className="font-mono text-xs text-gray-500 truncate" title={s.epic_code}>
                        {s.epic_code}
                      </span>
                      {s.unmapped_reason && (
                        <span className="badge bg-amber-50 text-amber-800" title={s.unmapped_reason}>
                          unmapped
                        </span>
                      )}
                      {removed && <RemovedBadge syncId={s.removed_sync_id} />}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      )}
      {data && data.total > 0 && <Pagination page={page} pageSize={PAGE_SIZE} total={data.total} onPage={setPage} disabled={loading} />}
    </div>
  );
}
