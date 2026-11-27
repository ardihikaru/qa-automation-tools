import { ReactNode, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronRight, Search, StickyNote } from 'lucide-react';
import { Coverage, Tree, TreeFeature, TreeModule, TreePart, TreeSubmodule, entityHref, getCoverage, getTree, listModules } from './api';
import { useDebounced, useLoad } from './hooks';
import { CodeLabel, MachineMark, Empty, ErrorNote, HideUntestedToggle, Pagination, PeekButton, ReleaseChip, RemovedBadge, SkeletonRows, UntestedRunButton } from './ui';
import { Paging, UNTESTED_RUN, testedFirst, useClampPage, useResetOnChange } from './paging';
import { inputCls } from './styles';
import { Target } from './EntityPanel';
import { CoverageCount, StatusBar } from './pages/kit';

interface Props {
  pid: number;
  paging: Paging;
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

// "Tested" comes from the tree's own counts, so ordering and hiding never wait for (or jump
// after) the coverage request.
const featTested = (f: TreeFeature) => f.test_cases > 0;
const subTested = (s: TreeSubmodule) => s.features.some(featTested);
const modTested = (m: TreeModule) => moduleFeatures(m).some(featTested);

/** Tested features first; untested ones dropped (hide) or kept for the caller to collapse. */
function orderModule(m: TreeModule, hide: boolean): TreeModule {
  const feats = (l: TreeFeature[]) => (hide ? l.filter(featTested) : testedFirst(l, featTested));
  const subs = testedFirst(m.submodules, subTested)
    .filter((s) => !hide || subTested(s))
    .map((s) => ({ ...s, features: feats(s.features) }));
  return { ...m, features_without_submodule: feats(m.features_without_submodule), submodules: subs };
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
          <MachineMark i18n={f.i18n} />
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

/**
 * Feature rows, with a long run of untested features at the end folded into one quiet line.
 * `open` is false only while that line is folded.
 */
function FeatureList({ features, fold, open, onUnfold, row }: { features: TreeFeature[]; fold: boolean; open: boolean; onUnfold: () => void; row: (f: TreeFeature) => ReactNode }) {
  const untested = features.filter((f) => !featTested(f));
  const folded = fold && !open && untested.length > UNTESTED_RUN;
  const shown = folded ? features.filter(featTested) : features;
  if (features.length === 0) return null;
  return (
    <ul>
      {shown.map(row)}
      {folded && (
        <li>
          <UntestedRunButton count={untested.length} noun={untested.length === 1 ? 'feature' : 'features'} onClick={onUnfold} className="rounded-md px-2" />
        </li>
      )}
    </ul>
  );
}

export default function ModulesTab({ pid, paging, refreshKey, selected, onOpen }: Props) {
  const [includeRemoved, setIncludeRemoved] = useState(false);
  const [hideUntested, setHideUntested] = useState(false);
  const [query, setQuery] = useState('');
  const q = useDebounced(query.trim().toLowerCase(), 200);
  const { page, size } = paging;
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
  // Unfolded runs of untested features, keyed by module or sub-module.
  const [openRuns, setOpenRuns] = useState<Set<string>>(new Set());

  useResetOnChange(JSON.stringify([q, includeRemoved, hideUntested]), () => paging.setPage(1, true));

  const toggle = (set: Set<string>, setter: (s: Set<string>) => void, key: string) => {
    const n = new Set(set);
    if (n.has(key)) n.delete(key);
    else n.add(key);
    setter(n);
  };

  const searched = useMemo(() => {
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

  // Tested modules first inside each part, untested ones dropped when hidden. Parts keep their order.
  const { parts, hiddenModules, hiddenFeatures } = useMemo(() => {
    let hm = 0;
    let hf = 0;
    const out: TreePart[] = [];
    for (const p of searched ?? []) {
      if (hideUntested) {
        for (const m of p.modules) {
          if (!modTested(m)) hm++;
          hf += moduleFeatures(m).filter((f) => !featTested(f)).length;
        }
      }
      const mods = testedFirst(p.modules, modTested)
        .filter((m) => !hideUntested || modTested(m))
        .map((m) => orderModule(m, hideUntested));
      if (mods.length > 0) out.push({ ...p, modules: mods });
    }
    return { parts: out, hiddenModules: hm, hiddenFeatures: hf };
  }, [searched, hideUntested]);

  // Part and module rows show their totals, not just what this page or "Hide untested" shows.
  const totals = useMemo(() => {
    const parts: Record<string, number> = {};
    const modules: Record<string, number> = {};
    for (const p of searched ?? []) {
      parts[p.code] = p.modules.length;
      for (const m of p.modules) modules[m.code] = moduleFeatures(m).length;
    }
    return { parts, modules };
  }, [searched]);

  // Pages hold `size` modules; each page groups its modules under their part headers again.
  const totalModules = parts.reduce((n, p) => n + p.modules.length, 0);
  useClampPage(paging, data ? totalModules : null);
  const paged = useMemo(() => {
    const from = (page - 1) * size;
    const to = from + size;
    const out: { part: TreePart; modules: TreeModule[] }[] = [];
    let i = 0;
    for (const p of parts) {
      const mods = p.modules.filter(() => {
        const k = i++;
        return k >= from && k < to;
      });
      if (mods.length > 0) out.push({ part: p, modules: mods });
    }
    return out;
  }, [parts, page, size]);

  const pageModules = paged.flatMap((g) => g.modules.map((m) => m.code));
  const c = cov.data;
  const fold = !q;
  const featureRow = (f: TreeFeature) => <FeatureRow key={f.code} pid={pid} f={f} cov={c?.features[f.code]} selected={selected === f.code} onOpen={onOpen} />;
  const hiddenLabel = `${[hiddenModules > 0 && `${hiddenModules.toLocaleString()} modules`, hiddenFeatures > 0 && `${hiddenFeatures.toLocaleString()} features`].filter(Boolean).join(' and ')} untested hidden`;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <label className="relative flex-1 min-w-[14rem] max-w-md">
          <Search className="w-4 h-4 text-gray-400 absolute left-2.5 top-1/2 -translate-y-1/2" aria-hidden="true" />
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Filter modules and features" aria-label="Filter modules and features" className={`${inputCls} w-full pl-8`} />
        </label>
        <HideUntestedToggle checked={hideUntested} onChange={setHideUntested} hidden={hiddenModules + hiddenFeatures} hiddenLabel={hiddenLabel} />
        <label className="inline-flex items-center gap-2 text-sm text-gray-700 min-h-8">
          <input type="checkbox" checked={includeRemoved} onChange={(e) => setIncludeRemoved(e.target.checked)} />
          Include removed
        </label>
        <span className="flex gap-2 ml-auto">
          <button type="button" className="btn btn-secondary text-xs px-2 py-1 min-h-8" onClick={() => setOpenModules(new Set([...openModules, ...pageModules]))} disabled={!data}>
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
      {loading && !data && <SkeletonRows rows={size + 1} label="Loading modules" />}
      {data && data.parts.length === 0 && <Empty>No modules yet. Run a sync from the product page.</Empty>}
      {data && data.parts.length > 0 && searched && searched.length === 0 && <Empty>Nothing matches “{query}”.</Empty>}
      {data && searched && searched.length > 0 && parts.length === 0 && <Empty>{q ? `No module matching “${query}” has test cases yet.` : 'No module has test cases yet.'}</Empty>}

      {/* One column of part cards. Two grid columns left a tall part's row height under its
          short neighbour (the gap under Part B); paging keeps each card short instead. */}
      {paged.length > 0 && (
        <div className={`space-y-3 ${loading ? 'opacity-60' : ''}`}>
          {paged.map(({ part: p, modules }) => {
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
                    <MachineMark i18n={p.i18n} />
                    <span className="ml-auto text-xs text-gray-500 shrink-0 tabular-nums">
                      {totals.parts[p.code] ?? p.modules.length} {(totals.parts[p.code] ?? p.modules.length) === 1 ? 'module' : 'modules'}
                    </span>
                  </Link>
                  <Cov c={c?.parts[p.code]} />
                  <PeekButton label={`Quick peek at part ${p.code}`} onClick={() => onOpen({ type: 'part', code: p.code })} />
                </div>
                {partOpen && (
                  <ul className="pl-3">
                    {modules.map((m) => {
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
                              <MachineMark i18n={m.i18n} />
                              {m.has_annotation && <StickyNote className="w-3.5 h-3.5 text-amber-500 shrink-0" aria-label="has a note" />}
                              <span className="ml-auto shrink-0 text-xs text-gray-500 tabular-nums">
                                {totals.modules[m.code] ?? feats.length} {(totals.modules[m.code] ?? feats.length) === 1 ? 'feature' : 'features'}
                              </span>
                            </Link>
                            <Cov c={c?.modules[m.code]} />
                            <PeekButton label={`Quick peek at module ${m.code}`} onClick={() => onOpen({ type: 'module', code: m.code })} />
                          </div>
                          {modOpen && (
                            <div className="pl-6 pb-2">
                              <FeatureList
                                features={m.features_without_submodule}
                                fold={fold}
                                open={openRuns.has(`m:${m.code}`)}
                                onUnfold={() => toggle(openRuns, setOpenRuns, `m:${m.code}`)}
                                row={featureRow}
                              />
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
                                      <div className="pl-5">
                                        <FeatureList
                                          features={s.features}
                                          fold={fold}
                                          open={openRuns.has(`s:${s.code}`)}
                                          onUnfold={() => toggle(openRuns, setOpenRuns, `s:${s.code}`)}
                                          row={featureRow}
                                        />
                                      </div>
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
      {data && totalModules > 0 && (
        <Pagination page={page} pageSize={size} total={totalModules} onPage={paging.setPage} onPageSize={paging.setSize} noun="Modules" disabled={loading} />
      )}
    </div>
  );
}
