// Shared by the product views: the fetch helper, API types and the form
// validation that mirrors the backend (ai-backend/internal/api/products.go,
// repopool.ValidateBranch). The server stays the authority.

export interface Check {
  name: string;
  ok: boolean;
  detail: string;
}
export interface VerifyReport {
  ok: boolean;
  checks: Check[];
  branch?: string;
  checked_at: string;
}
export interface BranchResult {
  name: string;
  status: 'ok' | 'error' | 'kept_dirty' | 'missing_remote';
  message: string;
}
export interface SyncReport {
  synced_at: string;
  results: BranchResult[];
}
export interface Repo {
  id: number;
  product_id: number;
  name: string;
  repo_url: string;
  root_dir: string;
  is_main: boolean;
  is_automation: boolean;
  is_central: boolean;
  default_branch: string;
  branches: string[];
  playwright_accessible: boolean;
  last_verified_at: string | null;
  verify_report: VerifyReport | null;
  sync_report: SyncReport | null;
  created_at: string;
  updated_at: string;
}
export interface Product {
  id: number;
  name: string;
  staging_url: string;
  allow_non_automation_work: boolean;
  /** Catalog format of the automation repo ("" = knowledge sync off). See PRODUCT_KNOWLEDGE_SYNC.md. */
  knowledge_format: string;
  // Legacy fields, mirrored from the automation repo.
  repo_url: string;
  root_dir: string;
  branch: string;
  playwright_accessible: boolean;
  last_verified_at: string | null;
  verify_report: VerifyReport | null;
  repos: Repo[];
  created_at?: string;
  updated_at?: string;
}
export interface SyncResponse {
  product: Product;
  results: BranchResult[];
}

export class ApiError extends Error {
  status: number;
  /** The parsed JSON error body, e.g. `{error, sync_id}` for a 409 on a running knowledge sync. */
  body: Record<string, unknown>;
  constructor(message: string, status: number, body: Record<string, unknown> = {}) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
  });
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(body?.error || `Request failed (${res.status})`, res.status, body && typeof body === 'object' ? body : {});
  return body as T;
}

// ---- validation (mirrors the backend) ----

export const MAX_BRANCHES = 10;
export const MAX_SLUG = 100;

const BRANCH_RE = /^[A-Za-z0-9][A-Za-z0-9._/-]*$/;

export const slugOf = (branch: string) => branch.replace(/\//g, '--');

export function branchNameProblem(b: string): string | null {
  if (!b) return 'Branch name is empty';
  if (b.length > 200 || !BRANCH_RE.test(b)) return `"${b}" is not a valid branch name`;
  if (b.includes('..') || b.includes('//') || b.endsWith('/') || b.endsWith('.') || b.endsWith('.lock')) {
    return `"${b}" is not a valid branch name`;
  }
  if (slugOf(b).length > MAX_SLUG) return `"${b}": branch name too long for a worktree (slug over ${MAX_SLUG})`;
  return null;
}

export interface BranchDraft {
  branches: string[];
  default: string;
}

/** `allowLegacyEmpty`: a repo that already has no branches may stay that way. */
export function branchProblem(d: BranchDraft, allowLegacyEmpty = false): string | null {
  if (d.branches.length === 0) return allowLegacyEmpty && !d.default ? null : 'Add at least one branch';
  if (d.branches.length > MAX_BRANCHES) return `At most ${MAX_BRANCHES} branches per repo`;
  if (!d.default || !d.branches.includes(d.default)) return 'Pick one default branch';
  const slugs = new Map<string, string>();
  for (const b of d.branches) {
    const bad = branchNameProblem(b);
    if (bad) return bad;
    const s = slugOf(b);
    if (slugs.has(s)) {
      return slugs.get(s) === b ? `Duplicate branch "${b}"` : `"${b}" and "${slugs.get(s)}" collide on the same worktree directory`;
    }
    slugs.set(s, b);
  }
  return null;
}

export const normalizeRepoUrl = (u: string) =>
  u.trim().toLowerCase().replace(/\/+$/, '').replace(/\.git$/, '').replace(/\/+$/, '');

const normDir = (d: string) => d.trim().replace(/^\/+|\/+$/g, '').toLowerCase();

export interface RepoFields {
  name: string;
  repo_url: string;
  root_dir: string;
}

/** Checks one repo's text fields against its siblings (`others`). Returns an error text or null. */
export function repoFieldProblem(f: RepoFields, others: Repo[]): string | null {
  const name = f.name.trim();
  if (!name) return 'Repo name is required';
  if (name.length > 100) return 'Repo name is at most 100 characters';
  if (!f.repo_url.trim()) return 'Repo link is required';
  if (!f.root_dir.trim()) return 'Root dir is required';
  if (f.root_dir.length > 200) return 'Root dir is at most 200 characters';
  if (f.root_dir.split('/').some((s) => s.toLowerCase().endsWith('.branches'))) {
    return 'Root dir cannot have a segment ending in ".branches" (reserved for worktrees)';
  }
  if (others.some((o) => o.name.toLowerCase() === name.toLowerCase())) return 'A repo with this name already exists in the product';
  if (others.some((o) => normalizeRepoUrl(o.repo_url) === normalizeRepoUrl(f.repo_url))) return 'This repository is already listed for the product';
  const dir = normDir(f.root_dir);
  if (others.some((o) => {
    const od = normDir(o.root_dir);
    return od === dir || dir.startsWith(`${od}/`) || od.startsWith(`${dir}/`);
  })) {
    return 'Root dir overlaps another repo of this product';
  }
  return null;
}

export const CHECK_LABELS: Record<string, string> = {
  root_dir: 'Root dir exists in the pool',
  git_repo: 'Is a git repository',
  origin_matches: 'Origin matches repo link',
  playwright: 'Playwright present',
  staging_url: 'Staging URL reachable',
};

export function mainRepo(p: Product): Repo | undefined {
  return p.repos?.find((r) => r.is_main);
}

export function automationRepo(p: Product): Repo | undefined {
  return p.repos?.find((r) => r.is_automation);
}
