import { KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { Plus } from 'lucide-react';
import { useAuth } from '../../auth/AuthContext';
import { usePageHeader } from '../../components/pageHeader';
import { Product, api } from '../products/api';
import {
  EntityDetail,
  Epic,
  StoryDetail,
  Summary,
  TYPE_LABEL,
  TestCaseDetail,
  errorText,
  canWriteKnowledge,
  getSummary,
  getTree,
  isEntityType,
  listEpics,
} from './api';
import Drawer from './Drawer';
import EntityPanel, { Target } from './EntityPanel';
import DraftModal, { DraftKind } from './DraftModal';
import ModulesTab from './ModulesTab';
import FeaturesTab from './FeaturesTab';
import StoriesTab from './StoriesTab';
import TestCasesTab from './TestCasesTab';
import SyncsTab from './SyncsTab';
import { ErrorNote, Loading, RelTime, ShaLink, SyncStatusChip } from './ui';
import { useKnowledgeSync } from './useKnowledgeSync';
import SyncButton from './SyncButton';
import { DEFAULT_PAGE_SIZE, Paging, parsePage, parseSize } from './paging';

const TABS = [
  { id: 'modules', label: 'Modules' },
  { id: 'features', label: 'Features' },
  { id: 'stories', label: 'Stories' },
  { id: 'test-cases', label: 'Test cases' },
  { id: 'syncs', label: 'Syncs' },
] as const;
type TabId = (typeof TABS)[number]['id'];
const isTab = (v: string | null): v is TabId => TABS.some((t) => t.id === v);

function parseTarget(v: string | null): Target | null {
  if (!v) return null;
  const i = v.indexOf(':');
  if (i < 1) return null;
  const type = v.slice(0, i);
  const code = v.slice(i + 1);
  return isEntityType(type) && code ? { type, code } : null;
}

export default function KnowledgeView() {
  const { id } = useParams();
  const pid = Number(id);
  const validId = Number.isInteger(pid) && pid > 0;
  const { user } = useAuth();
  const canWrite = canWriteKnowledge(user?.role ?? '');
  const [params, setParams] = useSearchParams();
  const tab: TabId = isTab(params.get('tab')) ? (params.get('tab') as TabId) : 'modules';
  const target = parseTarget(params.get('e'));

  const [product, setProduct] = useState<Product | null>(null);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [epics, setEpics] = useState<Epic[]>([]);
  const [features, setFeatures] = useState<{ code: string; name: string }[] | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [panelKey, setPanelKey] = useState(0);
  const [visited, setVisited] = useState<Set<TabId>>(() => new Set([tab]));
  const [draft, setDraft] = useState<{ kind: DraftKind; existing?: StoryDetail | TestCaseDetail } | null>(null);
  // The drawer keeps showing the last entity while it slides out.
  const [shown, setShown] = useState<Target | null>(target);
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});

  useEffect(() => {
    if (target) setShown(target);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target?.type, target?.code]);

  useEffect(() => {
    setVisited((v) => (v.has(tab) ? v : new Set(v).add(tab)));
  }, [tab]);

  const loadAll = useCallback(async () => {
    if (!validId) return;
    try {
      const [p, s] = await Promise.all([api<Product>(`/api/products/${pid}`), getSummary(pid)]);
      setProduct(p);
      setSummary(s);
      setLoadError(null);
    } catch (e) {
      setLoadError(errorText(e, 'Could not load the knowledge base'));
    }
  }, [pid, validId]);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  useEffect(() => {
    if (!validId || !summary?.configured) return;
    const ac = new AbortController();
    listEpics(pid, false, ac.signal)
      .then(setEpics)
      .catch(() => undefined);
    return () => ac.abort();
  }, [pid, validId, summary?.configured, refreshKey]);

  // Feature codes for the story filter and the story draft form: loaded on first need.
  const needFeatures = tab === 'stories' || draft?.kind === 'story';
  useEffect(() => {
    if (!needFeatures || features || !validId) return;
    const ac = new AbortController();
    getTree(pid, false, ac.signal)
      .then((t) =>
        setFeatures(
          t.parts.flatMap((p) => p.modules.flatMap((m) => [...m.features_without_submodule, ...m.submodules.flatMap((s) => s.features)])).map((f) => ({ code: f.code, name: f.name })),
        ),
      )
      .catch(() => undefined);
    return () => ac.abort();
  }, [needFeatures, features, pid, validId]);

  // Paging: `?size=` is shared by every tab; `?page=` belongs to the open tab. Other tabs keep
  // their page here and get it back in the URL when they are opened again.
  const size = parseSize(params.get('size'));
  const urlPage = parsePage(params.get('page'));
  const tabPages = useRef<Partial<Record<TabId, number>>>({});

  const setTab = (t: TabId) => {
    if (t === tab) return;
    tabPages.current[tab] = urlPage;
    const back = tabPages.current[t] ?? 1;
    delete tabPages.current[t];
    const n = new URLSearchParams(params);
    n.set('tab', t);
    if (back > 1) n.set('page', String(back));
    else n.delete('page');
    setParams(n, { replace: true });
  };

  const pagingFor = (t: TabId): Paging => ({
    page: t === tab ? urlPage : tabPages.current[t] ?? 1,
    size,
    setPage: (p, replace = false) => {
      if (t !== tab) {
        tabPages.current[t] = p;
        return;
      }
      setParams(
        (cur) => {
          const n = new URLSearchParams(cur);
          if (p > 1) n.set('page', String(p));
          else n.delete('page');
          return n;
        },
        { replace },
      );
    },
    setSize: (s) => {
      tabPages.current = {};
      setParams(
        (cur) => {
          const n = new URLSearchParams(cur);
          if (s === DEFAULT_PAGE_SIZE) n.delete('size');
          else n.set('size', String(s));
          n.delete('page');
          return n;
        },
        { replace: true },
      );
    },
  });

  const open = useCallback(
    (t: Target) => {
      const n = new URLSearchParams(params);
      n.set('e', `${t.type}:${t.code}`);
      setParams(n);
    },
    [params, setParams],
  );

  const close = useCallback(() => {
    const n = new URLSearchParams(params);
    n.delete('e');
    setParams(n, { replace: true });
  }, [params, setParams]);

  const onTabKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = TABS.findIndex((t) => t.id === tab);
    let next = -1;
    if (e.key === 'ArrowRight') next = (i + 1) % TABS.length;
    else if (e.key === 'ArrowLeft') next = (i - 1 + TABS.length) % TABS.length;
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = TABS.length - 1;
    if (next < 0) return;
    e.preventDefault();
    setTab(TABS[next].id);
    tabRefs.current[TABS[next].id]?.focus();
  };

  const mutated = useCallback(
    (deleted?: boolean) => {
      setRefreshKey((n) => n + 1);
      if (deleted) close();
    },
    [close],
  );

  function draftSaved(e: EntityDetail) {
    setDraft(null);
    setRefreshKey((n) => n + 1);
    setPanelKey((n) => n + 1);
    void getSummary(pid).then(setSummary).catch(() => undefined);
    open({ type: e.type, code: e.code });
  }

  const featureOptions = useMemo(() => features ?? [], [features]);
  const selectedCode = target?.code ?? null;

  // Sync from the top bar. A finished sync reloads the counts and the open tab.
  const sync = useKnowledgeSync(validId ? pid : 0, summary?.latest_sync ?? null, () => {
    if (validId) void getSummary(pid).then(setSummary).catch(() => undefined);
  });
  const syncStatus = sync.latest?.status;
  const prevStatus = useRef(syncStatus);
  useEffect(() => {
    if (prevStatus.current === 'running' && syncStatus && syncStatus !== 'running') setRefreshKey((n) => n + 1);
    prevStatus.current = syncStatus;
  }, [syncStatus]);

  const backTo = `/products?product=${pid}`;
  const productLabel = product?.name ?? (validId ? `Product #${pid}` : 'Product');
  const noRepo = !!summary && !summary.repo;
  usePageHeader({
    title: 'Knowledge base',
    titleHint: product ? `Knowledge base of ${product.name}` : 'Knowledge base',
    crumbs: [{ label: 'Products', to: '/products' }, ...(validId ? [{ label: productLabel, to: backTo }] : [])],
    actions:
      canWrite && summary?.configured ? (
        <SyncButton
          withText
          label={`Sync knowledge base of ${productLabel}`}
          busy={sync.busy}
          disabled={noRepo}
          reason={noRepo ? 'Set an automation repo first' : undefined}
          onClick={() => void sync.start()}
        />
      ) : undefined,
  });

  if (!validId) {
    return <ErrorNote>Unknown product.</ErrorNote>;
  }

  if (!summary) {
    return loadError ? <ErrorNote onRetry={() => void loadAll()}>{loadError}</ErrorNote> : <Loading />;
  }

  const latest = sync.latest;

  // Fixed overlays (drawer, modal) sit outside the space-y container, whose child margins
  // would otherwise offset them.
  return (
    <>
      <div className="space-y-4">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm text-gray-500 flex flex-wrap items-center gap-x-2 gap-y-1">
              <span>
                {summary.counts.features} features · {summary.counts.stories} stories · {summary.counts.test_cases} test cases · {Math.round(summary.coverage.pct)}% automated
              </span>
              {latest && (
                <span className="inline-flex items-center gap-2">
                  · <SyncStatusChip status={latest.status} /> sync #{latest.id} <RelTime at={latest.finished_at ?? latest.started_at} />
                  <ShaLink sha={latest.commit_sha} repoUrl={summary.repo?.url ?? latest.repo_url} />
                </span>
              )}
            </p>
          </div>
          {canWrite && summary.configured && (
            <div className="flex flex-wrap gap-2">
              <button type="button" className="btn btn-secondary text-sm inline-flex items-center gap-1.5 min-h-8" onClick={() => setDraft({ kind: 'story' })}>
                <Plus className="w-4 h-4" aria-hidden="true" /> New story draft
              </button>
              <button type="button" className="btn btn-secondary text-sm inline-flex items-center gap-1.5 min-h-8" onClick={() => setDraft({ kind: 'test_case' })}>
                <Plus className="w-4 h-4" aria-hidden="true" /> New test case draft
              </button>
            </div>
          )}
        </header>

        {sync.error && <ErrorNote>{sync.error}</ErrorNote>}
        {sync.note && (
          <p role="status" className="text-sm text-blue-800 bg-blue-50 border border-blue-100 rounded-lg px-3 py-2">
            {sync.note}
          </p>
        )}
        {latest?.status === 'failed' && latest.error && (
          <ErrorNote>
            Sync #{latest.id} failed: {latest.error}
          </ErrorNote>
        )}

        {!summary.configured ? (
          <div className="card p-6 text-sm text-gray-600">
            No catalog format is set for this product. Set one on the <Link to={backTo} className="text-primary-600 hover:underline">product page</Link>, then Sync.
          </div>
        ) : (
          <>
            <div role="tablist" aria-label="Knowledge base sections" className="flex gap-1 border-b border-gray-200 overflow-x-auto" onKeyDown={onTabKey}>
              {TABS.map((t) => {
                const active = t.id === tab;
                return (
                  <button
                    key={t.id}
                    ref={(el) => (tabRefs.current[t.id] = el)}
                    type="button"
                    role="tab"
                    id={`kb-tab-${t.id}`}
                    aria-selected={active}
                    aria-controls={`kb-panel-${t.id}`}
                    tabIndex={active ? 0 : -1}
                    onClick={() => setTab(t.id)}
                    className={`relative px-4 py-2 min-h-10 text-sm font-medium whitespace-nowrap ${active ? 'text-primary-700' : 'text-gray-600 hover:text-gray-900'}`}
                  >
                    {t.label}
                    <span
                      aria-hidden="true"
                      className={`absolute inset-x-2 -bottom-px h-0.5 rounded-full bg-primary-600 origin-center transition-[transform,opacity] duration-200 ease-out-strong ${
                        active ? 'opacity-100 scale-x-100' : 'opacity-0 scale-x-50'
                      }`}
                    />
                  </button>
                );
              })}
            </div>

            {TABS.map((t) =>
              visited.has(t.id) ? (
                <div key={t.id} role="tabpanel" id={`kb-panel-${t.id}`} aria-labelledby={`kb-tab-${t.id}`} hidden={t.id !== tab} className="kb-tab-enter">
                  {t.id === 'modules' && <ModulesTab pid={pid} paging={pagingFor('modules')} refreshKey={refreshKey} selected={target?.type === 'feature' ? selectedCode : null} onOpen={open} />}
                  {t.id === 'features' && (
                    <FeaturesTab pid={pid} paging={pagingFor('features')} releases={summary.enums.release ?? []} refreshKey={refreshKey} selected={target?.type === 'feature' ? selectedCode : null} onOpen={open} />
                  )}
                  {t.id === 'stories' && (
                    <StoriesTab pid={pid} paging={pagingFor('stories')} epics={epics} features={featureOptions} refreshKey={refreshKey} selected={target?.type === 'story' ? selectedCode : null} onOpen={open} />
                  )}
                  {t.id === 'test-cases' && (
                    <TestCasesTab pid={pid} paging={pagingFor('test-cases')} enums={summary.enums} epics={epics} refreshKey={refreshKey} selected={target?.type === 'test_case' ? selectedCode : null} onOpen={open} />
                  )}
                  {t.id === 'syncs' && <SyncsTab pid={pid} paging={pagingFor('syncs')} refreshKey={refreshKey} onOpen={open} />}
                </div>
              ) : null,
            )}
          </>
        )}
      </div>

      <Drawer open={!!target} title={shown ? `${TYPE_LABEL[shown.type]} ${shown.code}` : 'Details'} onClose={close}>
        {shown && (
          <EntityPanel
            pid={pid}
            target={shown}
            canWrite={canWrite}
            reloadKey={panelKey}
            onOpen={open}
            onEditDraft={(e) => setDraft({ kind: e.type as DraftKind, existing: e })}
            onMutated={mutated}
          />
        )}
      </Drawer>

      {draft && (
        <DraftModal
          pid={pid}
          kind={draft.kind}
          existing={draft.existing}
          enums={summary.enums}
          epics={epics}
          features={featureOptions}
          onClose={() => setDraft(null)}
          onSaved={draftSaved}
        />
      )}
    </>
  );
}
