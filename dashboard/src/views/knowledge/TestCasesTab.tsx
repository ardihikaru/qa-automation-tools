import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Bot, Search, StickyNote } from 'lucide-react';
import { Enums, Epic, Page, TestCase, entityHref, listTestCases } from './api';
import { useDebounced, useLoad } from './hooks';
import { CaseStatusChip, CodeLabel, Empty, ErrorNote, Pagination, SkeletonRows, PeekButton, RemovedBadge } from './ui';
import { inputCls, selectCls } from './styles';
import { Target } from './EntityPanel';
import { Paging, useClampPage, useResetOnChange } from './paging';


interface Props {
  pid: number;
  paging: Paging;
  enums: Enums;
  epics: Epic[];
  refreshKey: number;
  selected: string | null;
  onOpen: (t: Target) => void;
}

const flat = (s: string) => s.replace(/<br\s*\/?>/gi, ' ').replace(/\s+/g, ' ').trim();

export default function TestCasesTab({ pid, paging, enums, epics, refreshKey, selected, onOpen }: Props) {
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('');
  const [level, setLevel] = useState('');
  const [priority, setPriority] = useState('');
  const [epic, setEpic] = useState('');
  const [automated, setAutomated] = useState('');
  const [origin, setOrigin] = useState('');
  const [tag, setTag] = useState('');
  const [includeRemoved, setIncludeRemoved] = useState(false);
  const { page, size } = paging;
  const q = useDebounced(query.trim(), 300);
  const t = useDebounced(tag.trim(), 300);

  useResetOnChange(JSON.stringify([q, status, level, priority, epic, automated, origin, t, includeRemoved]), () => paging.setPage(1, true));

  const filter = { q, status, level, priority, epic, automated, origin, tag: t, include_removed: includeRemoved, page, page_size: size };
  const { data, loading, error, reload } = useLoad<Page<TestCase>>(`${pid}:${JSON.stringify(filter)}:${refreshKey}`, (s) => listTestCases(pid, filter, s));
  useClampPage(paging, data?.total);
  const filtered = !!(q || status || level || priority || epic || automated || origin || t);

  const enumSelect = (label: string, value: string, set: (v: string) => void, opts: string[]) => (
    <select value={value} onChange={(e) => set(e.target.value)} aria-label={label} className={selectCls}>
      <option value="">{label}: all</option>
      {opts.map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </select>
  );

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <label className="relative flex-1 min-w-[14rem]">
          <Search className="w-4 h-4 text-gray-400 absolute left-2.5 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search code or scenario" aria-label="Search test cases" className={`${inputCls} w-full pl-8`} />
        </label>
        {enumSelect('Status', status, setStatus, enums.status)}
        {enumSelect('Level', level, setLevel, enums.level)}
        {enumSelect('Priority', priority, setPriority, enums.priority)}
        <select value={epic} onChange={(e) => setEpic(e.target.value)} aria-label="Epic" className={`${selectCls} max-w-[14rem]`}>
          <option value="">All epics</option>
          {epics.map((e) => (
            <option key={e.code} value={e.code}>
              {e.code}
              {e.test_cases !== undefined ? ` (${e.test_cases})` : ''}
            </option>
          ))}
        </select>
        <select value={automated} onChange={(e) => setAutomated(e.target.value)} aria-label="Automated" className={selectCls}>
          <option value="">Automated: any</option>
          <option value="1">Automated</option>
          <option value="0">Not automated</option>
        </select>
        <select value={origin} onChange={(e) => setOrigin(e.target.value)} aria-label="Origin" className={selectCls}>
          <option value="">Synced + local</option>
          <option value="sync">Synced</option>
          <option value="local">Local drafts</option>
        </select>
        <input value={tag} onChange={(e) => setTag(e.target.value)} placeholder="Tag" aria-label="Tag" className={`${inputCls} w-28`} />
        <label className="inline-flex items-center gap-2 text-sm text-gray-700 min-h-8">
          <input type="checkbox" checked={includeRemoved} onChange={(e) => setIncludeRemoved(e.target.checked)} />
          Include removed
        </label>
      </div>

      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <SkeletonRows rows={size} label="Loading test cases" />}
      {data && data.items.length === 0 && <Empty>{filtered ? 'No test cases match these filters.' : 'No test cases yet.'}</Empty>}

      {data && data.items.length > 0 && (
        <div className={`card overflow-x-auto ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
          <table className="w-full min-w-[59rem] table-fixed text-sm">
            <colgroup>
              <col className="w-[11rem]" />
              <col />
              <col className="w-[10rem]" />
              <col className="w-[4.5rem]" />
              <col className="w-[4.5rem]" />
              <col className="w-[3rem]" />
              <col className="w-[8rem]" />
              <col className="w-[3rem]" />
            </colgroup>
            <thead className="bg-gray-50 text-left text-gray-600">
              <tr>
                <th className="px-3 py-2 font-medium">Code</th>
                <th className="px-3 py-2 font-medium">Scenario</th>
                <th className="px-3 py-2 font-medium">Story</th>
                <th className="px-3 py-2 font-medium">Level</th>
                <th className="px-3 py-2 font-medium">Priority</th>
                <th className="px-3 py-2 font-medium" title="Polarity">±</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-1 py-2 font-medium">
                  <span className="sr-only">Quick peek</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {data.items.map((c) => {
                const removed = !!c.removed_at;
                const scen = flat(c.scenario);
                return (
                  <tr key={c.code} className={`kb-row ${selected === c.code ? 'bg-primary-50' : ''}`}>
                    <td className="px-3 py-2">
                      <Link to={entityHref(pid, 'test_case', c.code)} className="flex items-center gap-1.5 min-w-0 min-h-8 text-left hover:underline">
                        <CodeLabel code={c.code} origin={c.origin} removed={removed} />
                        {c.annotation && <StickyNote className="w-3.5 h-3.5 text-amber-500 shrink-0" aria-label="has a note" />}
                      </Link>
                      {removed && <RemovedBadge syncId={c.removed_sync_id} />}
                    </td>
                    <td className="px-3 py-2 truncate" title={scen}>
                      <Link to={entityHref(pid, 'test_case', c.code)} tabIndex={-1} className={`hover:underline ${removed ? 'line-through text-gray-400' : 'text-gray-800'}`}>
                        {scen}
                      </Link>
                    </td>
                    <td className="px-3 py-2 font-mono text-xs text-gray-600 truncate" title={c.story_code ?? ''}>
                      {c.story_code || <span className="text-gray-400">—</span>}
                    </td>
                    <td className="px-3 py-2 text-xs">{c.level}</td>
                    <td className="px-3 py-2 text-xs">{c.priority}</td>
                    <td className="px-3 py-2 text-xs">{c.polarity === '-' ? '−' : c.polarity}</td>
                    <td className="px-3 py-2">
                      <span className="inline-flex items-center gap-1">
                        <CaseStatusChip status={c.status} />
                        {c.automated && c.status !== 'automated' && <Bot className="w-3.5 h-3.5 text-green-600" aria-label="automated" />}
                      </span>
                    </td>
                    <td className="px-1 py-1">
                      <PeekButton label={`Quick peek at test case ${c.code}`} active={selected === c.code} onClick={() => onOpen({ type: 'test_case', code: c.code })} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {data && data.total > 0 && <Pagination page={page} pageSize={size} total={data.total} onPage={paging.setPage} onPageSize={paging.setSize} noun="Test cases" disabled={loading} />}
    </div>
  );
}
