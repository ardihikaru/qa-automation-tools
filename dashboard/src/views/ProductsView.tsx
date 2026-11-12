import { FormEvent, useCallback, useEffect, useState } from 'react';
import { CheckCircle2, ChevronDown, ChevronRight, ExternalLink, GitBranch, Pencil, Plus, Trash2, XCircle, Download } from 'lucide-react';
import SubmitButton from '../components/SubmitButton';
import { ProductsIcon, RepoCheckIcon } from '../components/icons';

interface Check {
  name: string;
  ok: boolean;
  detail: string;
}
interface VerifyReport {
  ok: boolean;
  checks: Check[];
  branch?: string;
  checked_at: string;
}
interface Product {
  id: number;
  name: string;
  repo_url: string;
  staging_url: string;
  root_dir: string;
  branch: string;
  playwright_accessible: boolean;
  last_verified_at: string | null;
  verify_report: VerifyReport | null;
}
interface ProductForm {
  name: string;
  repo_url: string;
  staging_url: string;
  root_dir: string;
  branch: string;
}

const EMPTY_FORM: ProductForm = { name: '', repo_url: '', staging_url: '', root_dir: '', branch: '' };

const CHECK_LABELS: Record<string, string> = {
  root_dir: 'Root dir exists in the pool',
  git_repo: 'Is a git repository',
  origin_matches: 'Origin matches repo link',
  playwright: 'Playwright present',
  staging_url: 'Staging URL reachable',
};

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
  });
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body?.error || `Request failed (${res.status})`);
  return body as T;
}

function StatusChip({ p }: { p: Product }) {
  if (!p.last_verified_at) return <span className="badge bg-gray-100 text-gray-700">Unverified</span>;
  return p.playwright_accessible ? (
    <span className="badge badge-success">Accessible</span>
  ) : (
    <span className="badge badge-error">Not accessible</span>
  );
}

export default function ProductsView() {
  const [products, setProducts] = useState<Product[]>([]);
  const [poolEnabled, setPoolEnabled] = useState(true);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // One in-flight action per product, e.g. "12:verify".
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [editing, setEditing] = useState<{ id: number | null; form: ProductForm } | null>(null);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [branchFor, setBranchFor] = useState<{ p: Product; branch: string } | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await api<{ products: Product[]; pool_enabled: boolean }>('/api/products');
      setProducts(data.products);
      setPoolEnabled(data.pool_enabled);
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

  const isBusy = (id: number, action: string) => busy.has(`${id}:${action}`);
  const rowBusy = (id: number) => Array.from(busy).some((k) => k.startsWith(`${id}:`));

  async function runAction(p: Product, action: 'verify' | 'clone' | 'checkout', body?: unknown) {
    const key = `${p.id}:${action}`;
    if (rowBusy(p.id)) return; // one operation per product at a time
    setBusy((s) => new Set(s).add(key));
    setError(null);
    try {
      const updated = await api<Product>(`/api/products/${p.id}${action === 'verify' ? '/verify' : `/${action}`}`, {
        method: 'POST',
        body: body ? JSON.stringify(body) : undefined,
      });
      setProducts((list) => list.map((x) => (x.id === p.id ? updated : x)));
      setExpanded((s) => new Set(s).add(p.id));
      return true;
    } catch (e) {
      setError(`${p.name}: ${e instanceof Error ? e.message : `${action} failed`}`);
      return false;
    } finally {
      setBusy((s) => {
        const n = new Set(s);
        n.delete(key);
        return n;
      });
    }
  }

  async function remove(p: Product) {
    if (rowBusy(p.id)) return;
    if (!window.confirm(`Remove "${p.name}" from the list? The checkout on disk is kept.`)) return;
    setBusy((s) => new Set(s).add(`${p.id}:delete`));
    try {
      await api(`/api/products/${p.id}`, { method: 'DELETE' });
      setProducts((list) => list.filter((x) => x.id !== p.id));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed');
    } finally {
      setBusy((s) => {
        const n = new Set(s);
        n.delete(`${p.id}:delete`);
        return n;
      });
    }
  }

  async function submitForm(e: FormEvent) {
    e.preventDefault();
    if (!editing || saving) return;
    setSaving(true);
    setFormError(null);
    try {
      const saved = await api<Product>(editing.id ? `/api/products/${editing.id}` : '/api/products', {
        method: editing.id ? 'PUT' : 'POST',
        body: JSON.stringify(editing.form),
      });
      setProducts((list) =>
        editing.id ? list.map((x) => (x.id === saved.id ? saved : x)) : [...list, saved].sort((a, b) => a.name.localeCompare(b.name)),
      );
      setEditing(null);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Could not save');
    } finally {
      setSaving(false);
    }
  }

  async function submitBranch(e: FormEvent) {
    e.preventDefault();
    if (!branchFor) return;
    const ok = await runAction(branchFor.p, 'checkout', { branch: branchFor.branch.trim() });
    if (ok) setBranchFor(null);
  }

  const setField = (k: keyof ProductForm, v: string) =>
    setEditing((cur) => (cur ? { ...cur, form: { ...cur.form, [k]: v } } : cur));

  return (
    <div>
      <div className="flex items-center justify-between mb-6 gap-3">
        <div className="flex items-center gap-3">
          <ProductsIcon className="w-7 h-7 text-primary-600" />
          <div>
            <h2 className="text-2xl font-bold">Products</h2>
            <p className="text-sm text-gray-500">Repos and staging URLs Playwright tests, checked out on rnd-v3-vpn.</p>
          </div>
        </div>
        <button
          className="btn btn-primary flex items-center gap-2 shrink-0"
          onClick={() => {
            setFormError(null);
            setEditing({ id: null, form: EMPTY_FORM });
          }}
        >
          <Plus className="w-4 h-4" /> Add product
        </button>
      </div>

      {!poolEnabled && (
        <div className="mb-4 text-sm text-yellow-800 bg-yellow-50 border border-yellow-100 rounded-lg px-3 py-2">
          The repo pool is not configured (<code>REPO_POOL_DIR</code>), so Verify, Clone and Switch branch are unavailable.
        </div>
      )}
      {error && (
        <div role="alert" className="mb-4 text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">
          {error}
        </div>
      )}

      <div className="card overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead className="bg-gray-50 text-left text-gray-600">
            <tr>
              <th className="px-4 py-3 w-8" />
              <th className="px-4 py-3">Product</th>
              <th className="px-4 py-3">Staging URL</th>
              <th className="px-4 py-3">Root dir / branch</th>
              <th className="px-4 py-3">Status</th>
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
                  No products yet. Add one, then Clone and Verify it.
                </td>
              </tr>
            )}
            {products.map((p) => {
              const open = expanded.has(p.id);
              const disabled = rowBusy(p.id);
              return (
                <ProductRows
                  key={p.id}
                  p={p}
                  open={open}
                  toggle={() =>
                    setExpanded((s) => {
                      const n = new Set(s);
                      if (n.has(p.id)) n.delete(p.id);
                      else n.add(p.id);
                      return n;
                    })
                  }
                >
                  <SubmitButton
                    className="btn btn-secondary text-xs px-2 py-1"
                    loading={isBusy(p.id, 'verify')}
                    loadingLabel="Verifying…"
                    disabled={disabled || !poolEnabled}
                    onClick={() => runAction(p, 'verify')}
                    title="Check the checkout, origin, Playwright and staging URL"
                  >
                    <RepoCheckIcon className="w-4 h-4" /> Verify
                  </SubmitButton>
                  <SubmitButton
                    className="btn btn-secondary text-xs px-2 py-1"
                    loading={isBusy(p.id, 'clone')}
                    loadingLabel="Cloning…"
                    disabled={disabled || !poolEnabled}
                    onClick={() => runAction(p, 'clone')}
                    title="gh repo clone into the pool"
                  >
                    <Download className="w-4 h-4" /> Clone
                  </SubmitButton>
                  <button
                    className="btn btn-secondary text-xs px-2 py-1 inline-flex items-center gap-1 disabled:opacity-50 disabled:cursor-not-allowed"
                    disabled={disabled || !poolEnabled}
                    onClick={() => setBranchFor({ p, branch: p.branch })}
                    title="Switch branch"
                  >
                    <GitBranch className="w-4 h-4" /> Branch
                  </button>
                  <button
                    className="btn btn-secondary text-xs px-2 py-1 disabled:opacity-50"
                    disabled={disabled}
                    aria-label={`Edit ${p.name}`}
                    onClick={() => {
                      setFormError(null);
                      setEditing({
                        id: p.id,
                        form: { name: p.name, repo_url: p.repo_url, staging_url: p.staging_url, root_dir: p.root_dir, branch: p.branch },
                      });
                    }}
                  >
                    <Pencil className="w-4 h-4" />
                  </button>
                  <SubmitButton
                    className="btn btn-secondary text-xs px-2 py-1 text-red-600"
                    loading={isBusy(p.id, 'delete')}
                    disabled={disabled}
                    aria-label={`Remove ${p.name}`}
                    onClick={() => remove(p)}
                  >
                    <Trash2 className="w-4 h-4" />
                  </SubmitButton>
                </ProductRows>
              );
            })}
          </tbody>
        </table>
      </div>

      {editing && (
        <Modal title={editing.id ? 'Edit product' : 'Add product'} onClose={() => !saving && setEditing(null)}>
          <form onSubmit={submitForm} className="space-y-3">
            <Field label="Name" value={editing.form.name} onChange={(v) => setField('name', v)} placeholder="HISv3" required />
            <Field label="Repo link" value={editing.form.repo_url} onChange={(v) => setField('repo_url', v)} placeholder="https://github.com/owner/repo" required />
            <Field label="Main URL (staging)" value={editing.form.staging_url} onChange={(v) => setField('staging_url', v)} placeholder="https://staging.example.com" required />
            <Field label="Root dir (inside the repo pool)" value={editing.form.root_dir} onChange={(v) => setField('root_dir', v)} placeholder="owner/repo" hint="Relative to the pool, e.g. sismedika/hisv3. Created by Clone." required />
            <Field label="Branch (optional)" value={editing.form.branch} onChange={(v) => setField('branch', v)} placeholder="main" />
            {formError && <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">{formError}</div>}
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" className="btn btn-secondary" disabled={saving} onClick={() => setEditing(null)}>Cancel</button>
              <SubmitButton type="submit" className="btn btn-primary" loading={saving} loadingLabel="Saving…">Save</SubmitButton>
            </div>
          </form>
        </Modal>
      )}

      {branchFor && (
        <Modal title={`Switch branch: ${branchFor.p.name}`} onClose={() => !isBusy(branchFor.p.id, 'checkout') && setBranchFor(null)}>
          <form onSubmit={submitBranch} className="space-y-3">
            <Field label="Branch" value={branchFor.branch} onChange={(v) => setBranchFor({ ...branchFor, branch: v })} placeholder="main" required />
            <p className="text-xs text-gray-500">Runs git fetch, checkout and a fast-forward merge in the pool checkout.</p>
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" className="btn btn-secondary" disabled={isBusy(branchFor.p.id, 'checkout')} onClick={() => setBranchFor(null)}>Cancel</button>
              <SubmitButton type="submit" className="btn btn-primary" loading={isBusy(branchFor.p.id, 'checkout')} loadingLabel="Switching…" disabled={!branchFor.branch.trim()}>
                Switch
              </SubmitButton>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}

function ProductRows({ p, open, toggle, children }: { p: Product; open: boolean; toggle: () => void; children: React.ReactNode }) {
  const report = p.verify_report;
  return (
    <>
      <tr className="align-top">
        <td className="px-2 py-3">
          <button onClick={toggle} aria-expanded={open} aria-label={open ? 'Hide verification details' : 'Show verification details'} className="p-1 text-gray-500">
            {open ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
          </button>
        </td>
        <td className="px-4 py-3 max-w-[16rem]">
          <div className="font-medium text-gray-900 truncate" title={p.name}>{p.name}</div>
          <a href={p.repo_url} target="_blank" rel="noreferrer" className="text-xs text-primary-600 hover:underline inline-flex items-center gap-1 break-all">
            {p.repo_url.replace('https://github.com/', '')} <ExternalLink className="w-3 h-3 shrink-0" />
          </a>
        </td>
        <td className="px-4 py-3 max-w-[16rem]">
          <a href={p.staging_url} target="_blank" rel="noreferrer" className="text-primary-600 hover:underline break-all">{p.staging_url}</a>
        </td>
        <td className="px-4 py-3">
          <code className="text-xs break-all">{p.root_dir}</code>
          <div className="text-xs text-gray-500">{report?.branch || p.branch || '—'}</div>
        </td>
        <td className="px-4 py-3 whitespace-nowrap">
          <StatusChip p={p} />
          {p.last_verified_at && <div className="text-xs text-gray-500 mt-1">{new Date(p.last_verified_at).toLocaleString()}</div>}
        </td>
        <td className="px-4 py-3">
          <div className="flex flex-wrap gap-1 justify-end">{children}</div>
        </td>
      </tr>
      {open && (
        <tr className="bg-gray-50">
          <td />
          <td colSpan={5} className="px-4 py-3">
            {!report ? (
              <p className="text-sm text-gray-500">Not verified yet. Clone the repo (if needed), then press Verify.</p>
            ) : (
              <ul className="space-y-1">
                {report.checks.map((c) => (
                  <li key={c.name} className="flex items-start gap-2 text-sm">
                    {c.ok ? <CheckCircle2 className="w-4 h-4 text-green-600 mt-0.5 shrink-0" /> : <XCircle className="w-4 h-4 text-red-600 mt-0.5 shrink-0" />}
                    <span className="font-medium">{CHECK_LABELS[c.name] || c.name}</span>
                    {c.detail && <span className="text-gray-500 break-all">{c.detail}</span>}
                  </li>
                ))}
              </ul>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div role="dialog" aria-modal="true" aria-label={title} className="modal-enter bg-white rounded-xl shadow-xl w-full max-w-lg p-6 max-h-[90vh] overflow-y-auto">
        <h3 className="text-lg font-semibold mb-4">{title}</h3>
        {children}
      </div>
    </div>
  );
}

function Field({ label, value, onChange, placeholder, hint, required }: { label: string; value: string; onChange: (v: string) => void; placeholder?: string; hint?: string; required?: boolean }) {
  return (
    <label className="block">
      <span className="block text-sm font-medium text-gray-700 mb-1">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        required={required}
        className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
      />
      {hint && <span className="block text-xs text-gray-500 mt-1">{hint}</span>}
    </label>
  );
}
