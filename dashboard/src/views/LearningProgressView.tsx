import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Brain, Plus, RefreshCw } from 'lucide-react';
import { usePageHeader } from '../components/pageHeader';
import { useAuth } from '../auth/AuthContext';
import {
  BUSY_STATES,
  canWriteLearning,
  errorText,
  FINDING_KINDS,
  FINDING_STATUS_LABEL,
  FINDING_STATUSES,
  Finding,
  FindingStatus,
  isAbort,
  KIND_LABEL,
  listFindings,
  listProducts,
  listProposals,
  getTimeline,
  Page,
  ProductOption,
  Proposal,
  PROPOSAL_STATES,
  ProposalState,
  STATE_LABEL,
  TimelineItem,
} from './learning/api';
import { AddFindingDialog, FindingsTab, PoliciesTab, TimelineTab } from './learning/Tabs';

type Tab = 'timeline' | 'findings' | 'policies';
const TABS: { id: Tab; label: string }[] = [
  { id: 'timeline', label: 'Timeline' },
  { id: 'findings', label: 'Findings' },
  { id: 'policies', label: 'Policies' },
];

/**
 * /learning: what the QA tools learned from real automation work (LEARNING_LOOP.md). Findings
 * come from reviewer answers, repairs and repeated failures of case_create jobs, or are entered by
 * hand; a proposal turns findings into one policy file for the product's automation repo, a person
 * approves it, the tool opens the PR, and a merged PR makes the policy learned. The product filter
 * scopes everything; the URL keeps the filters.
 */
export default function LearningProgressView() {
  const { user } = useAuth();
  const canWrite = canWriteLearning(user?.role);
  const [params, setParams] = useSearchParams();
  const tab = (TABS.find((t) => t.id === params.get('tab'))?.id ?? 'timeline') as Tab;
  const productParam = params.get('product') ?? '';
  const fstatus = FINDING_STATUSES.includes(params.get('status') as FindingStatus) ? (params.get('status') as FindingStatus) : '';
  const kind = FINDING_KINDS.find((k) => k === params.get('kind')) ?? '';
  const pstate = PROPOSAL_STATES.includes(params.get('state') as ProposalState) ? (params.get('state') as ProposalState) : '';
  const q = params.get('q') ?? '';

  const [products, setProducts] = useState<ProductOption[] | null>(null);
  const [findings, setFindings] = useState<Page<Finding, FindingStatus> | null>(null);
  const [proposals, setProposals] = useState<Page<Proposal, ProposalState> | null>(null);
  const [allProposals, setAllProposals] = useState<Page<Proposal, ProposalState> | null>(null);
  const [timeline, setTimeline] = useState<TimelineItem[] | null>(null);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  const [adding, setAdding] = useState(false);
  const [qDraft, setQDraft] = useState(q);

  usePageHeader({ title: 'Learning' });

  const update = useCallback(
    (patch: Record<string, string>) => {
      const next = new URLSearchParams(params);
      for (const [k, v] of Object.entries(patch)) {
        if (v) next.set(k, v);
        else next.delete(k);
      }
      setParams(next, { replace: true });
    },
    [params, setParams],
  );

  // Products, and the default product: the first one with an automation repo.
  useEffect(() => {
    const ac = new AbortController();
    listProducts(ac.signal)
      .then((ps) => {
        setProducts(ps);
        if (!productParam) {
          const first = ps.find((p) => p.hasAutomation) ?? ps[0];
          update({ product: first ? String(first.id) : 'all' });
        }
      })
      .catch((e) => !isAbort(e) && setError(errorText(e)));
    return () => ac.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const pid = productParam === 'all' ? 0 : Number(productParam) || 0;
  const scoped = productParam !== '';

  useEffect(() => {
    if (!scoped) return;
    const ac = new AbortController();
    setError('');
    Promise.all([
      listFindings({ product_id: pid, status: fstatus, kind, q, limit: 200 }, ac.signal),
      listProposals({ product_id: pid, state: pstate, limit: 200 }, ac.signal),
      pstate ? listProposals({ product_id: pid, limit: 1 }, ac.signal) : Promise.resolve(null),
      getTimeline({ product_id: pid, limit: 200 }, ac.signal),
    ])
      .then(([f, p, all, t]) => {
        setFindings(f);
        setProposals(p);
        setAllProposals(all ?? p);
        setTimeline(t.items);
      })
      .catch((e) => !isAbort(e) && setError(errorText(e)));
    return () => ac.abort();
  }, [scoped, pid, fstatus, kind, pstate, q, reload]);

  // Poll while the server is drafting or publishing a visible proposal.
  const busy = (proposals?.items ?? []).some((p) => BUSY_STATES.includes(p.state));
  useEffect(() => {
    if (!busy) return;
    const h = window.setInterval(() => setReload((n) => n + 1), 10000);
    return () => window.clearInterval(h);
  }, [busy]);

  // The search box commits after a pause.
  useEffect(() => setQDraft(q), [q]);
  useEffect(() => {
    if (qDraft === q) return;
    const h = window.setTimeout(() => update({ q: qDraft.trim() }), 400);
    return () => window.clearTimeout(h);
  }, [qDraft, q, update]);

  const productName = useMemo(() => (pid ? products?.find((p) => p.id === pid)?.name ?? `product ${pid}` : 'all products'), [pid, products]);
  const refresh = () => setReload((n) => n + 1);
  const counts = allProposals?.counts;
  const tiles = [
    { label: 'New findings', value: findings?.counts.new, onClick: () => update({ tab: 'findings', status: 'new' }) },
    { label: 'Awaiting approval', value: counts?.awaiting_approval, onClick: () => update({ tab: 'policies', state: 'awaiting_approval' }) },
    { label: 'PRs open', value: counts?.pr_open, onClick: () => update({ tab: 'policies', state: 'pr_open' }) },
    { label: 'Learned policies', value: counts?.learned, onClick: () => update({ tab: 'policies', state: 'learned' }) },
  ];
  const loading = scoped && !error && (!findings || !proposals || !timeline);

  return (
    <div className="space-y-4 min-w-0">
      <header className="card">
        <div className="card-body space-y-3">
          <div className="flex flex-wrap items-start gap-3">
            <Brain className="w-5 h-5 text-primary-600 mt-0.5 shrink-0" aria-hidden="true" />
            <div className="min-w-[14rem] flex-1">
              <h1 className="text-lg font-semibold text-gray-900">Learning</h1>
              <p className="text-sm text-gray-600">
                What the QA tools learned from real automation work, and the policies merged into the product's automation repo. A merged policy is read by every
                later "Create automation" run.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {canWrite && (
                <button type="button" className="btn btn-secondary h-9 px-3 text-sm inline-flex items-center gap-1.5" onClick={() => setAdding(true)}>
                  <Plus className="w-4 h-4" aria-hidden="true" /> Add finding
                </button>
              )}
              <button type="button" className="btn btn-secondary h-9 px-3 text-sm inline-flex items-center gap-1.5" onClick={refresh} aria-label="Refresh">
                <RefreshCw className="w-4 h-4" aria-hidden="true" />
              </button>
            </div>
          </div>

          <div className="flex flex-wrap items-end gap-3" role="group" aria-label="Filters">
            <label className="text-sm text-gray-700 flex flex-col gap-1 min-w-0">
              Product
              <select
                className="h-9 rounded border border-gray-300 px-2 text-sm bg-white max-w-[16rem]"
                value={productParam || ''}
                onChange={(e) => update({ product: e.target.value })}
                disabled={!products}
              >
                {!productParam && <option value="">Loading…</option>}
                <option value="all">All products</option>
                {(products ?? []).map((p) => (
                  <option key={p.id} value={String(p.id)}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
            {tab !== 'policies' && (
              <>
                <label className="text-sm text-gray-700 flex flex-col gap-1">
                  Finding status
                  <select className="h-9 rounded border border-gray-300 px-2 text-sm bg-white" value={fstatus} onChange={(e) => update({ status: e.target.value })}>
                    <option value="">Any</option>
                    {FINDING_STATUSES.map((s) => (
                      <option key={s} value={s}>
                        {FINDING_STATUS_LABEL[s]}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="text-sm text-gray-700 flex flex-col gap-1">
                  Kind
                  <select className="h-9 rounded border border-gray-300 px-2 text-sm bg-white" value={kind} onChange={(e) => update({ kind: e.target.value })}>
                    <option value="">Any</option>
                    {FINDING_KINDS.map((k) => (
                      <option key={k} value={k}>
                        {KIND_LABEL[k]}
                      </option>
                    ))}
                  </select>
                </label>
              </>
            )}
            {tab !== 'findings' && (
              <label className="text-sm text-gray-700 flex flex-col gap-1">
                Policy state
                <select className="h-9 rounded border border-gray-300 px-2 text-sm bg-white" value={pstate} onChange={(e) => update({ state: e.target.value })}>
                  <option value="">Any</option>
                  {PROPOSAL_STATES.map((s) => (
                    <option key={s} value={s}>
                      {STATE_LABEL[s]}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {tab === 'findings' && (
              <label className="text-sm text-gray-700 flex flex-col gap-1 flex-1 min-w-[12rem]">
                Search
                <input
                  type="search"
                  className="h-9 rounded border border-gray-300 px-2 text-sm w-full"
                  placeholder="Title or lesson text"
                  value={qDraft}
                  maxLength={200}
                  onChange={(e) => setQDraft(e.target.value)}
                />
              </label>
            )}
          </div>
        </div>
      </header>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
        {tiles.map((t) => (
          <button key={t.label} type="button" onClick={t.onClick} className="card text-left px-4 py-3 hover:border-primary-300 focus-visible:ring-2 focus-visible:ring-primary-500">
            <span className="block text-xs text-gray-500">{t.label}</span>
            <span className="block text-2xl font-semibold tabular-nums text-gray-900">{t.value ?? '–'}</span>
          </button>
        ))}
      </div>

      <div className="card min-w-0">
        <div role="tablist" aria-label="Learning views" className="flex gap-1 border-b border-gray-200 px-2 overflow-x-auto">
          {TABS.map((t) => (
            <button
              key={t.id}
              role="tab"
              type="button"
              aria-selected={tab === t.id}
              className={`px-3 py-2.5 text-sm font-medium border-b-2 -mb-px whitespace-nowrap ${tab === t.id ? 'border-primary-600 text-primary-700' : 'border-transparent text-gray-600 hover:text-gray-900'}`}
              onClick={() => update({ tab: t.id === 'timeline' ? '' : t.id })}
            >
              {t.label}
              {t.id === 'findings' && findings ? ` (${findings.total})` : ''}
              {t.id === 'policies' && proposals ? ` (${proposals.total})` : ''}
            </button>
          ))}
        </div>
        <div className="card-body min-w-0" role="tabpanel">
          {error ? (
            <p role="alert" className="text-sm text-red-700">
              {error}{' '}
              <button type="button" className="underline" onClick={refresh}>
                Retry
              </button>
            </p>
          ) : loading ? (
            <p className="text-sm text-gray-500" aria-busy="true">
              Loading…
            </p>
          ) : tab === 'timeline' ? (
            <TimelineTab items={timeline ?? []} productName={productName} />
          ) : tab === 'findings' ? (
            <FindingsTab page={findings!} canWrite={canWrite} allProducts={!pid} onChanged={refresh} productName={productName} />
          ) : (
            <PoliciesTab page={proposals!} canWrite={canWrite} onChanged={refresh} productName={productName} />
          )}
        </div>
      </div>

      {adding && products && (
        <AddFindingDialog
          products={products}
          defaultProduct={pid || products.find((p) => p.hasAutomation)?.id || products[0]?.id || 0}
          onClose={() => setAdding(false)}
          onAdded={() => {
            setAdding(false);
            update({ tab: 'findings' });
            refresh();
          }}
        />
      )}
    </div>
  );
}
