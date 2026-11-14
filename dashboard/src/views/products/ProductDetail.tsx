import { FormEvent, useEffect, useRef, useState } from 'react';
import { Plus } from 'lucide-react';
import Field from '../../components/Field';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import RepoCard, { BranchesField, RepoAction } from './RepoCard';
import KnowledgeCard from '../knowledge/KnowledgeCard';
import GaosCard from './GaosCard';
import CaseResultsCard from './CaseResultsCard';
import {
  ApiError,
  BranchDraft,
  BranchResult,
  Product,
  Repo,
  SyncResponse,
  api,
  automationRepo,
  branchProblem,
  mainRepo,
  repoFieldProblem,
} from './api';

const FLAG_WARNING =
  'Sibling repos become writable for branch creation and PRs. Automation scripts normally live only in the automation repo.';

interface Props {
  product: Product;
  poolEnabled: boolean;
  canManage: boolean;
  canVerify: boolean;
  /** Admin and DevOps only; the server enforces it too. */
  canToggle: boolean;
  /** The server can push to GAOS (`gaos_configured` of the products list). */
  gaosConfigured: boolean;
  onChange: (p: Product) => void;
}

interface NewRepoForm {
  name: string;
  repo_url: string;
  root_dir: string;
  is_main: boolean;
  is_automation: boolean;
  is_central: boolean;
  draft: BranchDraft;
}
const emptyNewRepo = (isMain: boolean, isAutomation: boolean): NewRepoForm => ({
  name: '',
  repo_url: '',
  root_dir: '',
  is_main: isMain,
  is_automation: isAutomation,
  is_central: false,
  draft: { branches: ['main'], default: 'main' },
});

function describe(e: unknown, fallback: string): string {
  if (e instanceof ApiError) {
    if (e.status === 409 && /already running/i.test(e.message)) {
      return 'Another operation is running for this repo; try again';
    }
    if (e.status === 504) return 'Still running on the server; refresh in a minute';
    return e.message;
  }
  return e instanceof Error ? e.message : fallback;
}

export default function ProductDetail({ product, poolEnabled, canManage, canVerify, canToggle, gaosConfigured, onChange }: Props) {
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const inflight = useRef(false); // synchronous guard against rapid double clicks
  const reloadTimer = useRef<number | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);
  const [drafts, setDrafts] = useState<Record<number, BranchDraft>>({});
  const [lastResults, setLastResults] = useState<Record<number, { at?: string; results: BranchResult[] }>>({});
  const [meta, setMeta] = useState({ name: product.name, staging_url: product.staging_url });
  const [adding, setAdding] = useState(false);
  const [newRepo, setNewRepo] = useState<NewRepoForm>(() => emptyNewRepo(product.repos.length === 0, product.repos.length === 0));
  const [addError, setAddError] = useState<string | null>(null);
  const [editRepo, setEditRepo] = useState<{ repo: Repo; name: string; repo_url: string; root_dir: string } | null>(null);
  const [editError, setEditError] = useState<string | null>(null);

  useEffect(() => () => window.clearTimeout(reloadTimer.current), []);

  const repos = product.repos ?? [];
  const hasMain = repos.some((r) => r.is_main);
  const hasAutomation = repos.some((r) => r.is_automation);
  const mainR = mainRepo(product);
  const autoR = automationRepo(product);
  const showAddForm = adding || repos.length === 0 || !hasMain || !hasAutomation;
  const metaDirty = meta.name.trim() !== product.name || meta.staging_url.trim() !== product.staging_url;

  /** Runs one write at a time for the whole product; shows the error banner on failure. */
  async function run<T>(key: string, fn: () => Promise<T>, onOk: (r: T) => void): Promise<boolean> {
    if (inflight.current) return false;
    inflight.current = true;
    setBusyKey(key);
    setError(null);
    try {
      onOk(await fn());
      return true;
    } catch (e) {
      setError(describe(e, 'Action failed'));
      if (e instanceof ApiError && e.status === 504) {
        window.clearTimeout(reloadTimer.current);
        reloadTimer.current = window.setTimeout(() => {
          api<Product>(`/api/products/${product.id}`).then(onChange).catch(() => undefined);
        }, 30000);
      }
      return false;
    } finally {
      inflight.current = false;
      setBusyKey(null);
    }
  }

  const base = `/api/products/${product.id}`;
  const put = (body: unknown): RequestInit => ({ method: 'PUT', body: JSON.stringify(body) });

  const draftOf = (r: Repo): BranchDraft => drafts[r.id] ?? { branches: r.branches, default: r.default_branch };

  function onRepoAction(repo: Repo, action: RepoAction) {
    const rb = `${base}/repos/${repo.id}`;
    switch (action) {
      case 'verify':
        void run(`${repo.id}:verify`, () => api<Product>(`${rb}/verify`, { method: 'POST' }), onChange);
        break;
      case 'clone':
      case 'checkout':
        void run(
          `${repo.id}:${action}`,
          () => api<SyncResponse>(`${rb}/${action}`, { method: 'POST' }),
          (r) => {
            setLastResults((m) => ({ ...m, [repo.id]: { at: new Date().toISOString(), results: r.results ?? [] } }));
            onChange(r.product);
          },
        );
        break;
      case 'branches': {
        const d = draftOf(repo);
        void run(
          `${repo.id}:branches`,
          () => api<Product>(`${rb}/branches`, put({ branches: d.branches, default_branch: d.default })),
          (p) => {
            setDrafts((m) => {
              const n = { ...m };
              delete n[repo.id];
              return n;
            });
            onChange(p);
          },
        );
        break;
      }
      case 'main':
        void run(`${repo.id}:main`, () => api<Product>(rb, put({ is_main: true })), onChange);
        break;
      case 'automation':
        void run(`${repo.id}:automation`, () => api<Product>(rb, put({ is_automation: true })), onChange);
        break;
      case 'central':
        void run(`${repo.id}:central`, () => api<Product>(rb, put({ is_central: !repo.is_central })), onChange);
        break;
      case 'delete':
        if (!window.confirm(`Remove "${repo.name}" from this product? The checkout on disk is kept.`)) return;
        void run(`${repo.id}:delete`, () => api<Product>(rb, { method: 'DELETE' }), onChange);
        break;
      case 'edit':
        setEditError(null);
        setEditRepo({ repo, name: repo.name, repo_url: repo.repo_url, root_dir: repo.root_dir });
        break;
    }
  }

  function saveMeta(e: FormEvent) {
    e.preventDefault();
    const name = meta.name.trim();
    const staging = meta.staging_url.trim();
    if (!name || !staging) return setError('Name and staging URL are required');
    void run('product:save', () => api<Product>(base, put({ name, staging_url: staging })), onChange);
  }

  function toggleFlag() {
    const next = !product.allow_non_automation_work;
    if (next && !window.confirm(`${FLAG_WARNING}\n\nEnable "Allow non-automation work"?`)) return;
    void run('product:flag', () => api<Product>(base, put({ allow_non_automation_work: next })), onChange);
  }

  async function submitNewRepo(e: FormEvent) {
    e.preventDefault();
    const f = newRepo;
    const problem = repoFieldProblem(f, repos) || branchProblem(f.draft);
    if (problem) return setAddError(problem);
    setAddError(null);
    const ok = await run(
      'product:addrepo',
      () =>
        api<Product>(`${base}/repos`, {
          method: 'POST',
          body: JSON.stringify({
            name: f.name.trim(),
            repo_url: f.repo_url.trim(),
            root_dir: f.root_dir.trim(),
            is_main: f.is_main,
            is_automation: f.is_automation,
            is_central: f.is_central,
            branches: f.draft.branches,
            default_branch: f.draft.default,
          }),
        }),
      onChange,
    );
    if (ok) {
      setNewRepo(emptyNewRepo(false, false));
      setAdding(false);
    }
  }

  async function submitEditRepo(e: FormEvent) {
    e.preventDefault();
    if (!editRepo) return;
    const { repo } = editRepo;
    const fields = { name: editRepo.name.trim(), repo_url: editRepo.repo_url.trim(), root_dir: editRepo.root_dir.trim() };
    const problem = repoFieldProblem(fields, repos.filter((r) => r.id !== repo.id));
    if (problem) return setEditError(problem);
    const body: Record<string, string> = {};
    if (fields.name !== repo.name) body.name = fields.name;
    if (fields.repo_url !== repo.repo_url) body.repo_url = fields.repo_url;
    if (fields.root_dir !== repo.root_dir) body.root_dir = fields.root_dir;
    if (Object.keys(body).length === 0) return setEditRepo(null);
    setEditError(null);
    if (inflight.current) return;
    inflight.current = true;
    setBusyKey(`${repo.id}:edit`);
    try {
      onChange(await api<Product>(`${base}/repos/${repo.id}`, put(body)));
      setEditRepo(null);
    } catch (err) {
      setEditError(describe(err, 'Could not save'));
    } finally {
      inflight.current = false;
      setBusyKey(null);
    }
  }

  const switchTitle = canToggle ? undefined : 'Only Admin and DevOps can change this';

  return (
    <div className="space-y-4">
      {error && (
        <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">
          {error}
        </div>
      )}
      {!hasMain && (
        <div role="status" className="text-sm text-yellow-800 bg-yellow-50 border border-yellow-100 rounded-lg px-3 py-2">
          No main repo: add one.
        </div>
      )}
      {hasMain && !hasAutomation && (
        <div role="status" className="text-sm text-yellow-800 bg-yellow-50 border border-yellow-100 rounded-lg px-3 py-2">
          No automation repo: promote one (Make automation) or add one with Automation repo ticked.
        </div>
      )}
      {mainR && autoR && mainR.id !== autoR.id && (
        <div role="note" className="text-sm text-amber-800 bg-amber-50 border border-amber-100 rounded-lg px-3 py-2 break-words">
          Automation scripts live in <code>{autoR.name}</code>, not in the main repo <code>{mainR.name}</code>: Playwright verify, PR source and the
          product status apply to <code>{autoR.name}</code>. <code>{mainR.name}</code> (the AI base workspace) is read-only on the remote unless
          Allow non-automation work is on.
        </div>
      )}

      <form onSubmit={saveMeta} className="card p-4 space-y-3">
        <h3 className="text-base font-semibold">Name and staging URL</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Name" value={meta.name} onChange={(v) => setMeta((m) => ({ ...m, name: v }))} required />
          <Field label="Main URL (staging)" value={meta.staging_url} onChange={(v) => setMeta((m) => ({ ...m, staging_url: v }))} required />
        </div>
        <SubmitButton
          type="submit"
          className="btn btn-primary text-sm"
          loading={busyKey === 'product:save'}
          loadingLabel="Saving…"
          disabled={busyKey !== null || !canManage || !metaDirty}
          title={canManage ? undefined : 'Your role cannot manage products'}
        >
          Save
        </SubmitButton>
      </form>

      <div className="card p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="text-sm font-medium text-gray-800" id="flag-label">Allow non-automation work</div>
            <p className="text-xs text-gray-500 mt-1">
              Off by default. Sibling repos are read-only context; only the automation repo may create branches, push or open PRs.
            </p>
            {product.allow_non_automation_work && (
              <p role="note" className="text-xs text-amber-800 bg-amber-50 border border-amber-100 rounded px-2 py-1 mt-2">
                {FLAG_WARNING}
              </p>
            )}
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={product.allow_non_automation_work}
            aria-labelledby="flag-label"
            aria-busy={busyKey === 'product:flag'}
            disabled={!canToggle || busyKey !== null}
            title={switchTitle}
            onClick={toggleFlag}
            className={`relative shrink-0 w-11 h-6 rounded-full transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed ${
              product.allow_non_automation_work ? 'bg-amber-500' : 'bg-gray-300'
            }`}
          >
            <span
              className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform duration-150 ease-out-strong ${
                product.allow_non_automation_work ? 'translate-x-5' : 'translate-x-0'
              }`}
            />
          </button>
        </div>
      </div>

      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-base font-semibold">Repos ({repos.length})</h3>
          {hasMain && (
            <button
              type="button"
              className="btn btn-secondary text-xs px-2 py-1 inline-flex items-center gap-1 disabled:opacity-50"
              disabled={!canManage || busyKey !== null}
              title={canManage ? undefined : 'Your role cannot manage products'}
              onClick={() => setAdding((a) => !a)}
              aria-expanded={adding}
            >
              <Plus className="w-4 h-4" /> Add repo
            </button>
          )}
        </div>

        {repos.length === 0 && hasMain === false && <p className="text-sm text-gray-500">This product has no repos yet.</p>}

        {repos.map((r) => (
          <RepoCard
            key={r.id}
            repo={r}
            allowWork={product.allow_non_automation_work}
            draft={draftOf(r)}
            onDraft={(d) => setDrafts((m) => ({ ...m, [r.id]: d }))}
            results={lastResults[r.id]?.results ?? r.sync_report?.results ?? []}
            syncedAt={lastResults[r.id]?.at ?? r.sync_report?.synced_at}
            poolEnabled={poolEnabled}
            canManage={canManage}
            canVerify={canVerify}
            busyKey={busyKey}
            onAction={onRepoAction}
          />
        ))}

        {showAddForm && canManage && (
          <form onSubmit={submitNewRepo} className="card p-4 space-y-3" aria-label="Add repo">
            <h4 className="font-semibold">{hasMain ? 'Add repo' : 'Add the main repo'}</h4>
            <Field label="Repo name" value={newRepo.name} onChange={(v) => setNewRepo((f) => ({ ...f, name: v }))} placeholder="sut-ai" required />
            <Field label="Repo link" value={newRepo.repo_url} onChange={(v) => setNewRepo((f) => ({ ...f, repo_url: v }))} placeholder="https://github.com/owner/repo" required />
            <Field label="Root dir (inside the repo pool)" value={newRepo.root_dir} onChange={(v) => setNewRepo((f) => ({ ...f, root_dir: v }))} placeholder="owner/repo" hint="Relative to the pool. Created by Clone." required />
            <div className="flex flex-wrap gap-4 text-sm">
              <label className="inline-flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={newRepo.is_main}
                  disabled={!hasMain}
                  onChange={(e) => setNewRepo((f) => ({ ...f, is_main: e.target.checked }))}
                />
                Main repo
              </label>
              <label className="inline-flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={newRepo.is_automation}
                  disabled={repos.length === 0}
                  onChange={(e) => setNewRepo((f) => ({ ...f, is_automation: e.target.checked }))}
                />
                Automation repo
              </label>
              <label className="inline-flex items-center gap-2">
                <input type="checkbox" checked={newRepo.is_central} onChange={(e) => setNewRepo((f) => ({ ...f, is_central: e.target.checked }))} />
                Central repo
              </label>
            </div>
            {hasMain && newRepo.is_main && <p className="text-xs text-amber-800">The current main repo becomes a sibling.</p>}
            {hasAutomation && newRepo.is_automation && <p className="text-xs text-amber-800">The current automation repo becomes a sibling.</p>}
            {newRepo.is_central && repos.some((r) => r.is_central) && <p className="text-xs text-amber-800">The central flag moves from the current central repo.</p>}
            <BranchesField value={newRepo.draft} onChange={(d) => setNewRepo((f) => ({ ...f, draft: d }))} name="new-repo" disabled={busyKey !== null} />
            {addError && <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">{addError}</div>}
            <div className="flex justify-end gap-2">
              {hasMain && hasAutomation && (
                <button type="button" className="btn btn-secondary" disabled={busyKey !== null} onClick={() => setAdding(false)}>
                  Cancel
                </button>
              )}
              <SubmitButton type="submit" className="btn btn-primary" loading={busyKey === 'product:addrepo'} loadingLabel="Adding…" disabled={busyKey !== null}>
                Add repo
              </SubmitButton>
            </div>
          </form>
        )}
      </div>

      <KnowledgeCard product={product} canWrite={canVerify} canSetFormat={canToggle} onProductChange={onChange} />

      <GaosCard product={product} gaosConfigured={gaosConfigured} canToggle={canToggle} onChange={onChange} />

      <CaseResultsCard product={product} />

      {editRepo && (
        <Modal title={`Edit repo: ${editRepo.repo.name}`} onClose={() => busyKey === null && setEditRepo(null)}>
          <form onSubmit={submitEditRepo} className="space-y-3">
            <Field label="Repo name" value={editRepo.name} onChange={(v) => setEditRepo((s) => s && { ...s, name: v })} required />
            <Field label="Repo link" value={editRepo.repo_url} onChange={(v) => setEditRepo((s) => s && { ...s, repo_url: v })} required />
            <Field
              label="Root dir (inside the repo pool)"
              value={editRepo.root_dir}
              onChange={(v) => setEditRepo((s) => s && { ...s, root_dir: v })}
              hint="Changing the link or root dir clears this repo's verification. The old checkout stays on disk."
              required
            />
            {editError && <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">{editError}</div>}
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" className="btn btn-secondary" disabled={busyKey !== null} onClick={() => setEditRepo(null)}>Cancel</button>
              <SubmitButton type="submit" className="btn btn-primary" loading={busyKey === `${editRepo.repo.id}:edit`} loadingLabel="Saving…">Save</SubmitButton>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
