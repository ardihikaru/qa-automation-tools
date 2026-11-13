import { useMemo, useState } from 'react';
import { ChevronRight, Info, Search, StickyNote } from 'lucide-react';
import { Tree, TreeFeature, TreeModule, getTree } from './api';
import { useDebounced, useLoad } from './hooks';
import { CodeLabel, CoverageBar, Empty, ErrorNote, Loading, ReleaseChip, RemovedBadge } from './ui';
import { inputCls } from './styles';
import { Target } from './EntityPanel';

interface Props {
  pid: number;
  refreshKey: number;
  selected: string | null;
  onOpen: (t: Target) => void;
}

function matches(f: TreeFeature, q: string) {
  return f.code.toLowerCase().includes(q) || f.name.toLowerCase().includes(q);
}

function moduleFeatures(m: TreeModule): TreeFeature[] {
  return [...m.features_without_submodule, ...m.submodules.flatMap((s) => s.features)];
}

function Caret({ open }: { open: boolean }) {
  return <ChevronRight className={`w-4 h-4 text-gray-500 shrink-0 kb-caret ${open ? 'rotate-90' : ''}`} aria-hidden="true" />;
}

function FeatureRow({ f, selected, onOpen }: { f: TreeFeature; selected: boolean; onOpen: (t: Target) => void }) {
  const removed = !!f.removed_at;
  return (
    <li>
      <button
        type="button"
        onClick={() => onOpen({ type: 'feature', code: f.code })}
        className={`kb-row w-full text-left grid grid-cols-[minmax(0,1fr)_auto] md:grid-cols-[minmax(0,1fr)_7.5rem_4.5rem_4.5rem_8.5rem] items-center gap-x-3 gap-y-1 rounded-md px-2 py-1.5 min-h-8 ${
          selected ? 'bg-primary-50' : ''
        }`}
        aria-current={selected || undefined}
      >
        <span className="flex items-center gap-2 min-w-0">
          <CodeLabel code={f.code} origin={f.origin} removed={removed} />
          <span className={`truncate text-sm ${removed ? 'line-through text-gray-400' : 'text-gray-800'}`} title={f.name}>
            {f.name}
          </span>
          {f.has_annotation && <StickyNote className="w-3.5 h-3.5 text-amber-500 shrink-0" aria-label="has a note" />}
          {removed && <RemovedBadge />}
        </span>
        <span className="md:justify-self-start">
          <ReleaseChip status={f.release_status} asOf={f.status_as_of} />
        </span>
        <span className="hidden md:block text-xs tabular-nums text-gray-600" title="User stories">
          {f.stories} {f.stories === 1 ? 'story' : 'stories'}
        </span>
        <span className="hidden md:block text-xs tabular-nums text-gray-600" title="Test cases">
          {f.test_cases} TC
        </span>
        <span className="hidden md:block">
          {f.test_cases > 0 ? <CoverageBar value={f.coverage_pct} /> : <span className="text-xs text-gray-400">no test cases</span>}
        </span>
      </button>
    </li>
  );
}

export default function ModulesTab({ pid, refreshKey, selected, onOpen }: Props) {
  const [includeRemoved, setIncludeRemoved] = useState(false);
  const [query, setQuery] = useState('');
  const q = useDebounced(query.trim().toLowerCase(), 200);
  const { data, loading, error, reload } = useLoad<Tree>(`${pid}:${includeRemoved}:${refreshKey}`, (s) => getTree(pid, includeRemoved, s));
  // Parts start open, modules closed. A search opens everything that matches.
  const [closedParts, setClosedParts] = useState<Set<string>>(new Set());
  const [openModules, setOpenModules] = useState<Set<string>>(new Set());
  const [openSubs, setOpenSubs] = useState<Set<string>>(new Set());

  const toggle = (set: Set<string>, setter: (s: Set<string>) => void, key: string) => {
    const n = new Set(set);
    if (n.has(key)) n.delete(key);
    else n.add(key);
    setter(n);
  };

  const filtered = useMemo(() => {
    if (!data) return null;
    if (!q) return data.parts;
    return data.parts
      .map((p) => ({
        ...p,
        modules: p.modules
          .map((m) => {
            const modHit = m.code.toLowerCase().includes(q) || m.name.toLowerCase().includes(q);
            if (modHit) return m;
            return {
              ...m,
              features_without_submodule: m.features_without_submodule.filter((f) => matches(f, q)),
              submodules: m.submodules
                .map((s) => (s.name.toLowerCase().includes(q) ? s : { ...s, features: s.features.filter((f) => matches(f, q)) }))
                .filter((s) => s.features.length > 0),
            };
          })
          .filter((m) => moduleFeatures(m).length > 0 || m.code.toLowerCase().includes(q) || m.name.toLowerCase().includes(q)),
      }))
      .filter((p) => p.modules.length > 0);
  }, [data, q]);

  const allModules = data?.parts.flatMap((p) => p.modules.map((m) => m.code)) ?? [];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <label className="relative flex-1 min-w-[14rem] max-w-md">
          <Search className="w-4 h-4 text-gray-400 absolute left-2.5 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Filter modules and features" aria-label="Filter modules and features" className={`${inputCls} w-full pl-8`} />
        </label>
        <label className="inline-flex items-center gap-2 text-sm text-gray-700 min-h-8">
          <input type="checkbox" checked={includeRemoved} onChange={(e) => setIncludeRemoved(e.target.checked)} />
          Include removed
        </label>
        <span className="flex gap-2 ml-auto">
          <button type="button" className="btn btn-secondary text-xs px-2 py-1 min-h-8" onClick={() => setOpenModules(new Set(allModules))} disabled={!data}>
            Expand all
          </button>
          <button
            type="button"
            className="btn btn-secondary text-xs px-2 py-1 min-h-8"
            onClick={() => {
              setOpenModules(new Set());
              setOpenSubs(new Set());
            }}
            disabled={!data}
          >
            Collapse all
          </button>
        </span>
      </div>

      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <Loading />}
      {data && data.parts.length === 0 && <Empty>No modules yet. Run a sync from the product page.</Empty>}
      {filtered && data && data.parts.length > 0 && filtered.length === 0 && <Empty>Nothing matches “{query}”.</Empty>}

      {filtered && filtered.length > 0 && (
        <div className={`grid gap-3 2xl:grid-cols-2 items-start ${loading ? 'opacity-60' : ''}`}>
          {filtered.map((p) => {
            const partOpen = !!q || !closedParts.has(p.code);
            return (
              <section key={p.code} className="card p-2 min-w-0">
                <div className="flex items-center gap-1">
                  <button type="button" className="flex-1 min-w-0 flex items-center gap-2 px-2 py-2 min-h-8 text-left" aria-expanded={partOpen} onClick={() => toggle(closedParts, setClosedParts, p.code)}>
                    <Caret open={partOpen} />
                    <span className={`font-mono text-sm font-semibold ${p.removed_at ? 'line-through text-gray-400' : ''}`}>{p.code}</span>
                    <span className="font-semibold text-gray-900 truncate" title={p.name}>
                      {p.name}
                    </span>
                    <span className="ml-auto text-xs text-gray-500 shrink-0">{p.modules.length} modules</span>
                  </button>
                  <button type="button" className="p-1.5 min-h-8 text-gray-400 hover:text-primary-600 rounded" aria-label={`Details of part ${p.code}`} onClick={() => onOpen({ type: 'part', code: p.code })}>
                    <Info className="w-4 h-4" aria-hidden="true" />
                  </button>
                </div>
                {partOpen && (
                  <ul className="pl-3">
                    {p.modules.map((m) => {
                      const feats = moduleFeatures(m);
                      const modOpen = !!q || openModules.has(m.code);
                      const tc = feats.reduce((n, f) => n + f.test_cases, 0);
                      const auto = feats.reduce((n, f) => n + f.automated, 0);
                      return (
                        <li key={m.code} className="border-t border-gray-100 first:border-t-0">
                          <div className="flex items-center gap-1">
                            <button type="button" className="kb-row flex-1 min-w-0 flex items-center gap-2 px-2 py-1.5 min-h-8 rounded-md text-left" aria-expanded={modOpen} onClick={() => toggle(openModules, setOpenModules, m.code)}>
                              <Caret open={modOpen} />
                              <CodeLabel code={m.code} origin={m.origin} removed={!!m.removed_at} />
                              <span className={`truncate text-sm font-medium ${m.removed_at ? 'line-through text-gray-400' : 'text-gray-800'}`} title={m.name}>
                                {m.name}
                              </span>
                              {m.has_annotation && <StickyNote className="w-3.5 h-3.5 text-amber-500 shrink-0" aria-label="has a note" />}
                              <span className="ml-auto shrink-0 text-xs text-gray-500 tabular-nums">
                                {feats.length} features{tc > 0 ? ` · ${Math.round((auto * 100) / tc)}% automated` : ''}
                              </span>
                            </button>
                            <button type="button" className="p-1.5 min-h-8 text-gray-400 hover:text-primary-600 rounded" aria-label={`Details of module ${m.code}`} onClick={() => onOpen({ type: 'module', code: m.code })}>
                              <Info className="w-4 h-4" aria-hidden="true" />
                            </button>
                          </div>
                          {modOpen && (
                            <div className="pl-6 pb-2">
                              {m.features_without_submodule.length > 0 && (
                                <ul>
                                  {m.features_without_submodule.map((f) => (
                                    <FeatureRow key={f.code} f={f} selected={selected === f.code} onOpen={onOpen} />
                                  ))}
                                </ul>
                              )}
                              {m.submodules.map((s) => {
                                const subOpen = !!q || !openSubs.has(`x:${s.code}`);
                                return (
                                  <div key={s.code}>
                                    <button type="button" className="w-full flex items-center gap-2 px-2 py-1 min-h-8 text-left" aria-expanded={subOpen} onClick={() => toggle(openSubs, setOpenSubs, `x:${s.code}`)}>
                                      <Caret open={subOpen} />
                                      <span className={`text-sm text-gray-600 truncate ${s.removed_at ? 'line-through text-gray-400' : ''}`} title={s.name}>
                                        {s.name}
                                      </span>
                                      <span className="text-xs text-gray-400 shrink-0">({s.features.length})</span>
                                    </button>
                                    {subOpen && (
                                      <ul className="pl-5">
                                        {s.features.map((f) => (
                                          <FeatureRow key={f.code} f={f} selected={selected === f.code} onOpen={onOpen} />
                                        ))}
                                      </ul>
                                    )}
                                  </div>
                                );
                              })}
                              {feats.length === 0 && <p className="px-2 py-1 text-xs text-gray-500">No features.</p>}
                            </div>
                          )}
                        </li>
                      );
                    })}
                  </ul>
                )}
              </section>
            );
          })}
        </div>
      )}
    </div>
  );
}
