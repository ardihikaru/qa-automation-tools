import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Search, StickyNote } from 'lucide-react';
import { Epic, Page, StoryItem, entityHref, listStories } from './api';
import { useDebounced, useLoad } from './hooks';
import { CodeLabel, Empty, ErrorNote, Pagination, SkeletonRows, PeekButton, RemovedBadge } from './ui';
import { CoverageCount, StatusBar } from './pages/kit';
import { inputCls, selectCls } from './styles';
import { Target } from './EntityPanel';
import { Paging, useClampPage, useResetOnChange } from './paging';


interface Props {
  pid: number;
  paging: Paging;
  epics: Epic[];
  features: { code: string; name: string }[];
  refreshKey: number;
  selected: string | null;
  onOpen: (t: Target) => void;
}

export default function StoriesTab({ pid, paging, epics, features, refreshKey, selected, onOpen }: Props) {
  const [query, setQuery] = useState('');
  const [epic, setEpic] = useState('');
  const [feature, setFeature] = useState('');
  const [tag, setTag] = useState('');
  const [origin, setOrigin] = useState('');
  const [coverage, setCoverage] = useState('');
  const [includeRemoved, setIncludeRemoved] = useState(false);
  const { page, size } = paging;
  const q = useDebounced(query.trim(), 300);
  const f = useDebounced(feature.trim(), 300);
  const t = useDebounced(tag.trim(), 300);

  useResetOnChange(JSON.stringify([q, epic, f, t, origin, coverage, includeRemoved]), () => paging.setPage(1, true));

  const filter = { q, epic, feature: f, tag: t, origin, coverage, include_removed: includeRemoved, page, page_size: size };
  const { data, loading, error, reload } = useLoad<Page<StoryItem>>(`${pid}:${JSON.stringify(filter)}:${refreshKey}`, (s) => listStories(pid, filter, s));
  useClampPage(paging, data?.total);
  const filtered = !!(q || epic || f || t || origin || coverage);

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
        <select value={coverage} onChange={(e) => setCoverage(e.target.value)} aria-label="Coverage" className={selectCls}>
          <option value="">Any coverage</option>
          <option value="untested">No test cases yet</option>
          <option value="none,partial">Not automated yet</option>
          <option value="full">Fully automated</option>
        </select>
        <label className="inline-flex items-center gap-2 text-sm text-gray-700 min-h-8">
          <input type="checkbox" checked={includeRemoved} onChange={(e) => setIncludeRemoved(e.target.checked)} />
          Include removed
        </label>
      </div>

      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <SkeletonRows rows={size} label="Loading stories" />}
      {data && data.items.length === 0 && <Empty>{filtered ? 'No stories match these filters.' : 'No user stories yet.'}</Empty>}

      {data && data.items.length > 0 && (
        <div className={`card overflow-hidden ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
          <ul className="divide-y divide-gray-100">
            {data.items.map((s) => {
              const removed = !!s.removed_at;
              return (
                <li key={s.code} className={`flex items-center gap-1 pr-2 ${selected === s.code ? 'bg-primary-50' : ''}`}>
                  <Link
                    to={entityHref(pid, 'story', s.code)}
                    className="kb-row flex-1 min-w-0 grid grid-cols-[minmax(0,1fr)] md:grid-cols-[12rem_minmax(0,1fr)_9rem] xl:grid-cols-[12rem_minmax(0,1fr)_9rem_8rem_8.5rem] items-center gap-x-4 gap-y-1 px-4 py-2.5 min-h-8"
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
                    <StatusBar coverage={s.coverage} className="hidden xl:flex w-full" />
                    <span className="hidden xl:block text-right">
                      <CoverageCount coverage={s.coverage} />
                    </span>
                  </Link>
                  <PeekButton label={`Quick peek at story ${s.code}`} active={selected === s.code} onClick={() => onOpen({ type: 'story', code: s.code })} />
                </li>
              );
            })}
          </ul>
        </div>
      )}
      {data && data.total > 0 && <Pagination page={page} pageSize={size} total={data.total} onPage={paging.setPage} onPageSize={paging.setSize} noun="Stories" disabled={loading} />}
    </div>
  );
}
