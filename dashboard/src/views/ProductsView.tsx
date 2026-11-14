import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { ExternalLink, Plus, Settings2, Trash2 } from 'lucide-react';
import SubmitButton from '../components/SubmitButton';
import Modal from '../components/Modal';
import Field from '../components/Field';
import { usePageHeader } from '../components/pageHeader';
import { useAuth } from '../auth/AuthContext';
import ProductDetail from './products/ProductDetail';
import KnowledgeCell from './products/KnowledgeCell';
import { canSetFormat, canWriteKnowledge } from './knowledge/api';
import { BranchesField } from './products/RepoCard';
import { BranchDraft, Product, api, branchProblem, automationRepo, mainRepo, repoFieldProblem } from './products/api';

interface ProductForm {
  name: string;
  staging_url: string;
  repo_name: string;
  repo_url: string;
  root_dir: string;
  draft: BranchDraft;
}

const EMPTY_FORM: ProductForm = {
  name: '',
  staging_url: '',
  repo_name: 'main',
  repo_url: '',
  root_dir: '',
  draft: { branches: ['main'], default: 'main' },
};

function StatusChip({ p }: { p: Product }) {
  const auto = automationRepo(p);
  const main = mainRepo(p);
  if (!auto) return <span className="badge badge-error">No automation repo</span>;
  const autoChip = !auto.last_verified_at ? (
    <span className="badge bg-gray-100 text-gray-700">Unverified</span>
  ) : auto.playwright_accessible ? (
    <span className="badge badge-success">Accessible</span>
  ) : (
    <span className="badge badge-error">Not accessible</span>
  );
  if (!main || main.id === auto.id) return autoChip;
  const mainOk = main.verify_report?.ok ?? main.playwright_accessible;
  const mainChip = !main.last_verified_at ? (
    <span className="badge bg-gray-100 text-gray-700">Main: Unverified</span>
  ) : mainOk ? (
    <span className="badge badge-success">Main: OK</span>
  ) : (
    <span className="badge badge-error">Main: Failing</span>
  );
  return (
    <span className="inline-flex flex-wrap gap-1">
      {autoChip}
      {mainChip}
    </span>
  );
}

export default function ProductsView() {
  const { user } = useAuth();
  // Mirrors the API (products.go productRole, canToggleWork); the server is the authority.
  const role = user?.role || '';
  const canManage = ['Admin', 'DevOps', 'Developer'].includes(role);
  const canVerify = canManage || ['QA', 'QC'].includes(role);
  const canToggle = ['Admin', 'DevOps'].includes(role);
  const canSyncKnowledge = canWriteKnowledge(role);
  const canSetUpKnowledge = canSetFormat(role);
  const [products, setProducts] = useState<Product[]>([]);
  const [poolEnabled, setPoolEnabled] = useState(true);
  const [gaosConfigured, setGaosConfigured] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // `?product=<id>` opens that product's detail (the knowledge base links back this way).
  const [params, setParams] = useSearchParams();
  const [detailId, setDetailId] = useState<number | null>(() => {
    const n = Number(params.get('product'));
    return Number.isInteger(n) && n > 0 ? n : null;
  });
  useEffect(() => {
    if (params.has('product')) {
      const n = new URLSearchParams(params);
      n.delete('product');
      setParams(n, { replace: true });
    }
  }, [params, setParams]);
  const [removing, setRemoving] = useState<number | null>(null);
  const [editing, setEditing] = useState<ProductForm | null>(null);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await api<{ products: Product[]; pool_enabled: boolean; gaos_configured?: boolean }>('/api/products');
      setProducts(data.products);
      setPoolEnabled(data.pool_enabled);
      setGaosConfigured(!!data.gaos_configured);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load products');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  // Every response replaces the product in the list.
  const replaceProduct = useCallback((p: Product) => {
    setProducts((list) => list.map((x) => (x.id === p.id ? p : x)));
  }, []);

  async function remove(p: Product) {
    if (removing !== null) return;
    if (!window.confirm(`Remove "${p.name}" and its repos from the list? The checkouts on disk are kept.`)) return;
    setRemoving(p.id);
    setError(null);
    try {
      await api(`/api/products/${p.id}`, { method: 'DELETE' });
      setProducts((list) => list.filter((x) => x.id !== p.id));
      if (detailId === p.id) setDetailId(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed');
    } finally {
      setRemoving(null);
    }
  }

  async function submitForm(e: FormEvent) {
    e.preventDefault();
    if (!editing || saving) return;
    const f = editing;
    const problem =
      (!f.name.trim() || !f.staging_url.trim() ? 'Name and staging URL are required' : null) ||
      repoFieldProblem({ name: f.repo_name, repo_url: f.repo_url, root_dir: f.root_dir }, []) ||
      branchProblem(f.draft);
    if (problem) return setFormError(problem);
    setSaving(true);
    setFormError(null);
    try {
      const saved = await api<Product>('/api/products', {
        method: 'POST',
        body: JSON.stringify({
          name: f.name.trim(),
          staging_url: f.staging_url.trim(),
          repos: [
            {
              name: f.repo_name.trim(),
              repo_url: f.repo_url.trim(),
              root_dir: f.root_dir.trim(),
              is_main: true,
              is_automation: true,
              is_central: false,
              branches: f.draft.branches,
              default_branch: f.draft.default,
            },
          ],
        }),
      });
      setProducts((list) => [...list, saved].sort((a, b) => a.name.localeCompare(b.name)));
      setEditing(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Could not save');
    } finally {
      setSaving(false);
    }
  }

  const setField = (k: 'name' | 'staging_url' | 'repo_name' | 'repo_url' | 'root_dir', v: string) =>
    setEditing((cur) => (cur ? { ...cur, [k]: v } : cur));

  const detail = detailId === null ? undefined : products.find((p) => p.id === detailId);

  // The detail is state on the same route, so the "Products" crumb closes it instead of navigating.
  usePageHeader(
    detail
      ? { title: detail.name, crumbs: [{ label: 'Products', to: '/products', onClick: () => setDetailId(null) }] }
      : detailId !== null && loading
        ? { title: 'Product', crumbs: [{ label: 'Products', to: '/products', onClick: () => setDetailId(null) }] }
        : null,
  );

  return (
    <div>
      {!detail && (
        <div className="flex flex-wrap items-center justify-between mb-6 gap-3">
          <p className="text-sm text-gray-500 min-w-0">Repos, branches and staging URLs Playwright tests, checked out on rnd-v3-vpn.</p>
          <button
            className="btn btn-primary flex items-center gap-2 shrink-0 disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={!canManage}
            title={canManage ? undefined : 'Your role cannot manage products'}
            onClick={() => {
              setFormError(null);
              setEditing(EMPTY_FORM);
            }}
          >
            <Plus className="w-4 h-4" /> Add product
          </button>
        </div>
      )}

      {!poolEnabled && (
        <div className="mb-4 text-sm text-yellow-800 bg-yellow-50 border border-yellow-100 rounded-lg px-3 py-2">
          The repo pool is not configured (<code>REPO_POOL_DIR</code>), so Verify, Clone and Sync branches are unavailable.
        </div>
      )}
      {error && (
        <div role="alert" className="mb-4 text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">
          {error}
        </div>
      )}

      {detail ? (
        <ProductDetail
          key={detail.id}
          product={detail}
          poolEnabled={poolEnabled}
          canManage={canManage}
          canVerify={canVerify}
          canToggle={canToggle}
          gaosConfigured={gaosConfigured}
          onChange={replaceProduct}
        />
      ) : (
        <div className="card overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead className="bg-gray-50 text-left text-gray-600">
              <tr>
                <th className="px-4 py-3">Product</th>
                <th className="px-4 py-3 hidden 2xl:table-cell">Staging URL</th>
                <th className="px-4 py-3">Automation repo / branch</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3 2xl:min-w-[22rem]">Knowledge base</th>
                <th className="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {loading && (
                <tr>
                  <td colSpan={6} className="px-4 py-8 text-center text-gray-500">Loading…</td>
                </tr>
              )}
              {!loading && products.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-4 py-10 text-center text-gray-500">
                    No products yet. Add one, then open it to Clone and Verify its repos.
                  </td>
                </tr>
              )}
              {products.map((p) => {
                const auto = automationRepo(p);
                const main = mainRepo(p);
                return (
                  <tr key={p.id} className="align-top">
                    <td className="px-4 py-3 max-w-[14rem] 2xl:max-w-[16rem]">
                      <div className="font-medium text-gray-900 truncate" title={p.name}>{p.name}</div>
                      <a href={p.staging_url} target="_blank" rel="noreferrer" className="2xl:hidden block text-xs text-primary-600 hover:underline truncate" title={p.staging_url}>
                        {p.staging_url.replace(/^https?:\/\//, '')}
                      </a>
                      {p.allow_non_automation_work && <span className="badge bg-amber-100 text-amber-800">Non-automation work on</span>}
                      {p.gaos?.integrated && (
                        <span
                          className={`badge ml-1 ${p.gaos.last_status === 'failed' ? 'bg-red-100 text-red-700' : p.gaos.last_status === 'partial' ? 'bg-amber-100 text-amber-800' : 'bg-indigo-100 text-indigo-800'}`}
                          title={`Integrated with GAOS (${p.gaos.product_id}). Last sync: ${p.gaos.last_status || 'not yet'}${p.gaos.last_sync_at ? ` at ${new Date(p.gaos.last_sync_at).toLocaleString()}` : ''}`}
                        >
                          GAOS
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 max-w-[16rem] hidden 2xl:table-cell">
                      <a href={p.staging_url} target="_blank" rel="noreferrer" className="text-primary-600 hover:underline break-all">{p.staging_url}</a>
                    </td>
                    <td className="px-4 py-3 max-w-[16rem]">
                      {auto ? (
                        <>
                          <a href={auto.repo_url} target="_blank" rel="noreferrer" className="text-xs text-primary-600 hover:underline inline-flex items-center gap-1 break-all">
                            {auto.repo_url.replace('https://github.com/', '')} <ExternalLink className="w-3 h-3 shrink-0" />
                          </a>
                          <div className="text-xs text-gray-500 break-all">
                            <code>{auto.root_dir}</code> · {auto.default_branch || '—'} · {p.repos?.length ?? 0} {(p.repos?.length ?? 0) === 1 ? 'repo' : 'repos'}
                          </div>
                          {main && main.id !== auto.id && (
                            <div className="text-xs text-gray-400 break-all" title="Main repo (AI base workspace)">Main: {main.name}</div>
                          )}
                        </>
                      ) : (
                        <span className="text-xs text-gray-500">No automation repo: add one</span>
                      )}
                    </td>
                    <td className="px-4 py-3 whitespace-nowrap">
                      <StatusChip p={p} />
                      {auto?.last_verified_at && <div className="text-xs text-gray-500 mt-1">{new Date(auto.last_verified_at).toLocaleString()}</div>}
                    </td>
                    <td className="px-4 py-3">
                      <KnowledgeCell product={p} canSync={canSyncKnowledge} canSetUp={canSetUpKnowledge} onOpenDetails={() => setDetailId(p.id)} />
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1 justify-end">
                        <button className="btn btn-secondary text-xs px-2 py-1 inline-flex items-center gap-1" onClick={() => setDetailId(p.id)}>
                          <Settings2 className="w-4 h-4" /> Details
                        </button>
                        <SubmitButton
                          className="btn btn-secondary text-xs px-2 py-1 text-red-600"
                          loading={removing === p.id}
                          disabled={removing !== null || !canManage}
                          aria-label={`Remove ${p.name}`}
                          onClick={() => remove(p)}
                        >
                          <Trash2 className="w-4 h-4" />
                        </SubmitButton>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {editing && (
        <Modal title="Add product" onClose={() => !saving && setEditing(null)}>
          <form onSubmit={submitForm} className="space-y-3">
            <Field label="Name" value={editing.name} onChange={(v) => setField('name', v)} placeholder="HISv3" required />
            <Field label="Main URL (staging)" value={editing.staging_url} onChange={(v) => setField('staging_url', v)} placeholder="https://staging.example.com" required />
            <p className="text-xs text-gray-500">The first repo is both the main (AI workspace) and the automation repo. Change roles in the product detail. Add central and sibling repos from the product details.</p>
            <Field label="Main repo name" value={editing.repo_name} onChange={(v) => setField('repo_name', v)} placeholder="main" required />
            <Field label="Repo link" value={editing.repo_url} onChange={(v) => setField('repo_url', v)} placeholder="https://github.com/owner/repo" required />
            <Field label="Root dir (inside the repo pool)" value={editing.root_dir} onChange={(v) => setField('root_dir', v)} placeholder="owner/repo" hint="Relative to the pool, e.g. sismedika/hisv3. Created by Clone." required />
            <BranchesField value={editing.draft} onChange={(d) => setEditing((cur) => (cur ? { ...cur, draft: d } : cur))} name="new-product" disabled={saving} />
            {formError && <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">{formError}</div>}
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" className="btn btn-secondary" disabled={saving} onClick={() => setEditing(null)}>Cancel</button>
              <SubmitButton type="submit" className="btn btn-primary" loading={saving} loadingLabel="Saving…">Save</SubmitButton>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
