import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight, Search, StickyNote } from 'lucide-react';
import { Coverage, Tree, TreeFeature, TreeModule, entityHref, getCoverage, getTree, listModules } from './api';
import { useDebounced, useLoad } from './hooks';
import { CodeLabel, Empty, ErrorNote, Loading, PeekButton, ReleaseChip, RemovedBadge } from './ui';
import { inputCls } from './styles';
import { Target } from './EntityPanel';
import { CoverageCount, StatusBar } from './pages/kit';

interface Props {
  pid: number;
  refreshKey: number;
  selected: string | null;
  onOpen: (t: Target) => void;
}

type CovMap = Record<string, Coverage>;

function matches(f: TreeFeature, q: string) {
  return f.code.toLowerCase().includes(q) || f.name.toLowerCase().includes(q);
}

function moduleFeatures(m: TreeModule): TreeFeature[] {
  return [...m.features_without_submodule, ...m.submodules.flatMap((s) => s.features)];
}

function Caret({ open }: { open: boolean }) {
  return <ChevronRight className={`w-4 h-4 text-gray-500 shrink-0 kb-caret ${open ? 'rotate-90' : ''}`} aria-hidden="true" />;
}

function Toggle({ open, label, onClick }: { open: boolean; label: string; onClick: () => void }) {
  return (
    <button type="button" className="p-1.5 min-h-8 min-w-8 inline-flex items-center justify-center rounded shrink-0" aria-expanded={open} aria-label={`${open ? 'Collapse' : 'Expand'} ${label}`} onClick={onClick}>
      <Caret open={open} />
    </button>
  );
}

/** Bar + count, hidden on narrow screens where the row has no room. */
function Cov({ c }: { c: Coverage | undefined }) {
  return (
    <span className="hidden sm:flex items-center gap-3 shrink-0">
      <StatusBar coverage={c} className="w-28" />
      <span className="w-32 text-right">
        <CoverageCount coverage={c} />
      </span>
    </span>
  );
}

function FeatureRow({ pid, f, cov, selected, onOpen }: { pid: number; f: TreeFeature; cov: Coverage | undefined; selected: boolean; onOpen: (t: Target) => void }) {
  const removed = !!f.removed_at;
  return (
    <li className={`flex items-center gap-1 rounded-md ${selected ? 'bg-primary-50' : ''}`}>
      <Link
        to={entityHref(pid, 'feature', f.code)}
        className="kb-row flex-1 min-w-0 grid grid-cols-[minmax(0,1fr)_auto] md:grid-cols-[minmax(0,1fr)_7.5rem_4.5rem_auto] items-center gap-x-3 gap-y-1 rounded-md px-2 py-1.5 min-h-8"
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
        <Cov c={cov} />
      </Link>
      <PeekButton label={`Quick peek at feature ${f.code}`} active={selected} onClick={() => onOpen({ type: 'feature', code: f.code })} />
    </li>
  );
}

export default function ModulesTab({ pid, refreshKey, selected, onOpen }: Props) {
  const [includeRemoved, setIncludeRemoved] = useState(false);
  const [query, setQuery] = useState('');
  const q = useDebounced(query.trim().toLowerCase(), 200);
  const { data, loading, error, reload } = useLoad<Tree>(`${pid}:${includeRemoved}:${refreshKey}`, (s) => getTree(pid, includeRemoved, s));
  // Coverage is decoration: a failure leaves the bars empty rather than blocking the tree.
  const cov = useLoad<{ parts: CovMap; modules: CovMap; features: CovMap }>(`${pid}:cov:${refreshKey}`, async (s) => {
    const [parts, modules, features] = await Promise.all([
      getCoverage(pid, 'part', {}, s),
      listModules(pid, false, s).then((l) => Object.fromEntries(l.map((m) => [m.code, m.coverage]))),
      getCoverage(pid, 'feature', {}, s),
    ]);
    return { parts, modules, features };
  });
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
  const c = cov.data;

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
                  <Toggle open={partOpen} label={`part ${p.code}`} onClick={() => toggle(closedParts, setClosedParts, p.code)} />
                  <Link to={entityHref(pid, 'part', p.code)} className="kb-row flex-1 min-w-0 flex items-center gap-2 px-2 py-2 min-h-8 rounded-md">
                    <span className={`font-mono text-sm font-semibold ${p.removed_at ? 'line-through text-gray-400' : ''}`}>{p.code}</span>
                    <span className="font-semibold text-gray-900 truncate" title={p.name}>
                      {p.name}
                    </span>
                    <span className="ml-auto text-xs text-gray-500 shrink-0 tabular-nums">{p.modules.length} modules</span>
                  </Link>
                  <Cov c={c?.parts[p.code]} />
                  <PeekButton label={`Quick peek at part ${p.code}`} onClick={() => onOpen({ type: 'part', code: p.code })} />
                </div>
                {partOpen && (
                  <ul className="pl-3">
                    {p.modules.map((m) => {
                      const feats = moduleFeatures(m);
                      const modOpen = !!q || openModules.has(m.code);
                      return (
                        <li key={m.code} className="border-t border-gray-100 first:border-t-0">
                          <div className="flex items-center gap-1">
                            <Toggle open={modOpen} label={`module ${m.code}`} onClick={() => toggle(openModules, setOpenModules, m.code)} />
                            <Link to={entityHref(pid, 'module', m.code)} className="kb-row flex-1 min-w-0 flex items-center gap-2 px-2 py-1.5 min-h-8 rounded-md">
                              <CodeLabel code={m.code} origin={m.origin} removed={!!m.removed_at} />
                              <span className={`truncate text-sm font-medium ${m.removed_at ? 'line-through text-gray-400' : 'text-gray-800'}`} title={m.name}>
                                {m.name}
                              </span>
                              {m.has_annotation && <StickyNote className="w-3.5 h-3.5 text-amber-500 shrink-0" aria-label="has a note" />}
                              <span className="ml-auto shrink-0 text-xs text-gray-500 tabular-nums">{feats.length} features</span>
                            </Link>
                            <Cov c={c?.modules[m.code]} />
                            <PeekButton label={`Quick peek at module ${m.code}`} onClick={() => onOpen({ type: 'module', code: m.code })} />
                          </div>
                          {modOpen && (
                            <div className="pl-6 pb-2">
                              {m.features_without_submodule.length > 0 && (
                                <ul>
                                  {m.features_without_submodule.map((f) => (
                                    <FeatureRow key={f.code} pid={pid} f={f} cov={c?.features[f.code]} selected={selected === f.code} onOpen={onOpen} />
                                  ))}
                                </ul>
                              )}
                              {m.submodules.map((s) => {
                                const subOpen = !!q || !openSubs.has(`x:${s.code}`);
                                return (
                                  <div key={s.code}>
                                    <div className="flex items-center gap-1">
                                      <Toggle open={subOpen} label={`sub-module ${s.name}`} onClick={() => toggle(openSubs, setOpenSubs, `x:${s.code}`)} />
                                      <Link to={entityHref(pid, 'submodule', s.code)} className="kb-row min-w-0 flex items-center gap-2 px-2 py-1 min-h-8 rounded-md">
                                        <span className={`text-sm text-gray-600 truncate ${s.removed_at ? 'line-through text-gray-400' : ''}`} title={s.name}>
                                          {s.name}
                                        </span>
                                        <span className="text-xs text-gray-400 shrink-0 tabular-nums">({s.features.length})</span>
                                      </Link>
                                    </div>
                                    {subOpen && (
                                      <ul className="pl-5">
                                        {s.features.map((f) => (
                                          <FeatureRow key={f.code} pid={pid} f={f} cov={c?.features[f.code]} selected={selected === f.code} onOpen={onOpen} />
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
