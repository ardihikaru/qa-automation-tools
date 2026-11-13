import { useEffect, useState } from 'react';
import { Search } from 'lucide-react';
import { FeatureItem, ModuleItem, Page, listFeatures, listModules } from './api';
import { useDebounced, useLoad } from './hooks';
import { Empty, ErrorNote, Loading, Pagination, PeekButton, ReleaseChip } from './ui';
import { inputCls, selectCls } from './styles';
import { Target } from './EntityPanel';
import { CoverageRow } from './pages/kit';

const PAGE_SIZE = 50;

const COVERAGE_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: 'Any coverage' },
  { value: 'untested', label: 'No test cases yet' },
  { value: 'none,partial', label: 'Not automated yet' },
  { value: 'none', label: 'None automated' },
  { value: 'partial', label: 'Partly automated' },
  { value: 'full', label: 'Fully automated' },
];

interface Props {
  pid: number;
  releases: string[];
  refreshKey: number;
  selected: string | null;
  onOpen: (t: Target) => void;
}

export default function FeaturesTab({ pid, releases, refreshKey, selected, onOpen }: Props) {
  const [query, setQuery] = useState('');
  const [module, setModule] = useState('');
  const [release, setRelease] = useState('');
  const [coverage, setCoverage] = useState('');
  const [page, setPage] = useState(1);
  const q = useDebounced(query.trim(), 300);

  useEffect(() => setPage(1), [q, module, release, coverage]);

  const modules = useLoad<ModuleItem[]>(`${pid}:modules:${refreshKey}`, (s) => listModules(pid, false, s));
  const filter = { q, module, release, coverage, page, page_size: PAGE_SIZE };
  const { data, loading, error, reload } = useLoad<Page<FeatureItem>>(`${pid}:${JSON.stringify(filter)}:${refreshKey}`, (s) => listFeatures(pid, filter, s));
  const filtered = !!(q || module || release || coverage);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <label className="relative flex-1 min-w-[14rem]">
          <Search className="w-4 h-4 text-gray-400 absolute left-2.5 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search code or name" aria-label="Search features" className={`${inputCls} w-full pl-8`} />
        </label>
        <select value={module} onChange={(e) => setModule(e.target.value)} aria-label="Module" className={`${selectCls} max-w-[18rem]`}>
          <option value="">All modules</option>
          {(modules.data ?? []).map((m) => (
            <option key={m.code} value={m.code}>
              {m.code} {m.name.length > 40 ? `${m.name.slice(0, 40)}…` : m.name}
            </option>
          ))}
        </select>
        <select value={release} onChange={(e) => setRelease(e.target.value)} aria-label="Release status" className={selectCls}>
          <option value="">Any release</option>
          {releases.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
        <select value={coverage} onChange={(e) => setCoverage(e.target.value)} aria-label="Coverage" className={selectCls}>
          {COVERAGE_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <Loading />}
      {data && data.items.length === 0 && <Empty>{filtered ? 'No features match these filters.' : 'No features yet. Run a sync from the product page.'}</Empty>}

      {data && data.items.length > 0 && (
        <ul className={`rounded-lg border border-gray-200 bg-white divide-y divide-gray-100 ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
          {data.items.map((f) => (
            <li key={f.code} className={`flex items-center gap-1 pr-1 ${selected === f.code ? 'bg-primary-50' : ''}`}>
              <div className="flex-1 min-w-0">
                <CoverageRow
                  pid={pid}
                  type="feature"
                  code={f.code}
                  name={f.name}
                  origin={f.origin}
                  removed={!!f.removed_at}
                  coverage={f.coverage}
                  noted={!!f.annotation}
                  extra={
                    <span className="flex items-center gap-3">
                      <span className="hidden lg:inline font-mono text-xs text-gray-500" title="Module">
                        {f.module_code}
                      </span>
                      <ReleaseChip status={f.release_status} asOf={f.status_as_of} />
                    </span>
                  }
                />
              </div>
              <PeekButton label={`Quick peek at feature ${f.code}`} active={selected === f.code} onClick={() => onOpen({ type: 'feature', code: f.code })} />
            </li>
          ))}
        </ul>
      )}
      {data && data.total > 0 && <Pagination page={page} pageSize={PAGE_SIZE} total={data.total} onPage={setPage} disabled={loading} />}
    </div>
  );
}
