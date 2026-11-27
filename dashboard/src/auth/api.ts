// Thin fetch wrappers around /api/auth/*. Centralised so views never call the
// endpoints directly and so we always include credentials (cookie).
//
// There is no global 401 handler. This file only throws a typed AuthError
// (or returns null from me() on 401); each caller decides what a 401 means.
// AuthContext treats a 401 from me() or stopImpersonation() as "signed out".

export type Language = 'en' | 'id';
export const normalizeLanguage = (v: unknown): Language => (v === 'id' ? 'id' : 'en');

export interface AuthUser {
  id: string;
  username: string;
  fullName?: string;
  email?: string;
  whatsApp?: string;
  odooID?: string;
  role: string;
  isLead: boolean;
  active: boolean;
  mustChangePassword: boolean;
  /** Display language of the knowledge base; absent (older server) means 'en'. UI chrome stays English. */
  language?: Language;
  // Present only while impersonating: the real, signed-in user acting as
  // this one. Absent (not null) otherwise.
  impersonator?: Impersonator;
}

export interface Impersonator {
  id: string;
  username: string;
  fullName?: string;
  role: string;
}

// Row from GET /api/users (Admin/Service only). Includes inactive users and
// the caller; never includes credentials.
export interface UserSummary {
  id: string;
  username: string;
  fullName?: string;
  role: string;
  active: boolean;
}

// Shared shape of /api/auth/me, login, impersonate and impersonate/stop.
export interface LoginResponse {
  user: AuthUser;
  mustChangePassword: boolean;
}

export class AuthError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function authFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const res = await fetch(path, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init.headers || {}),
    },
  });
  return res;
}

function readError(res: Response, fallback: string): Promise<AuthError> {
  return res
    .json()
    .catch(() => ({ error: fallback }))
    .then((body: { error?: string }) => new AuthError(body.error || fallback, res.status));
}

export async function login(username: string, password: string): Promise<LoginResponse> {
  const res = await authFetch('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    throw await readError(res, 'Invalid username or password');
  }
  return res.json();
}

export async function logout(): Promise<void> {
  await authFetch('/api/auth/logout', { method: 'POST' });
}

export async function me(): Promise<LoginResponse | null> {
  const res = await authFetch('/api/auth/me');
  if (res.status === 401) return null;
  if (!res.ok) throw await readError(res, 'Could not load session');
  return res.json();
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  const res = await authFetch('/api/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ currentPassword, newPassword }),
  });
  if (!res.ok) {
    throw await readError(res, 'Could not change password');
  }
}

export async function setLanguage(language: Language): Promise<void> {
  const res = await authFetch('/api/auth/language', {
    method: 'PUT',
    body: JSON.stringify({ language }),
  });
  if (!res.ok) throw await readError(res, 'Could not save the language');
}

export async function listUsers(): Promise<UserSummary[]> {
  const res = await authFetch('/api/users');
  if (!res.ok) throw await readError(res, 'Could not load users');
  const body = await res.json();
  return Array.isArray(body) ? body : [];
}

export async function impersonate(username: string): Promise<LoginResponse> {
  const res = await authFetch('/api/auth/impersonate', {
    method: 'POST',
    body: JSON.stringify({ username }),
  });
  if (!res.ok) throw await readError(res, 'Could not log in as that user');
  return res.json();
}

export async function stopImpersonation(): Promise<LoginResponse> {
  const res = await authFetch('/api/auth/impersonate/stop', { method: 'POST' });
  if (!res.ok) throw await readError(res, 'Could not return to your account');
  return res.json();
}

// Role → Tailwind classes for the badge pill. Mirrors the palette from
// the implementation plan; falls back to slate for unknown roles.
// Keep in sync with normalizeRole AND upsertServiceUser (`Service`) in
// ai-backend/internal/seed/seed_users.go.
const ROLE_BADGE: Record<string, string> = {
  Admin: 'bg-slate-100 text-slate-700 ring-slate-200',
  Developer: 'bg-indigo-100 text-indigo-700 ring-indigo-200',
  DevOps: 'bg-violet-100 text-violet-700 ring-violet-200',
  QA: 'bg-emerald-100 text-emerald-700 ring-emerald-200',
  QC: 'bg-amber-100 text-amber-700 ring-amber-200',
  'UI/UX Designer': 'bg-rose-100 text-rose-700 ring-rose-200',
  Service: 'bg-cyan-100 text-cyan-700 ring-cyan-200',
};

// Real roles allowed to impersonate (mirrors auth.ImpersonatorRoles). UI hint
// only; the backend enforces it.
export const IMPERSONATOR_ROLES: readonly string[] = ['Admin', 'Service'];

export function roleBadgeClass(role: string): string {
  return ROLE_BADGE[role] || ROLE_BADGE.Admin;
}
