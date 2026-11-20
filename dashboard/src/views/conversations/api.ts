// Stored AI conversations (docs/designs/AI_CONVERSATIONS.md, ai-backend/internal/api/conversations.go).
import type { AuthUser } from '../../auth/api';
import { api } from '../products/api';

export interface Conversation {
  id: number;
  kind: 'job' | 'sync' | 'call' | string;
  ref: string;
  product_id: number | null;
  product_name: string;
  feature: string;
  title: string;
  provider: string;
  model: string;
  message_count: number;
  bytes: number;
  created_at: string;
  updated_at: string;
  archived_at: string | null;
  archived_by_name: string;
}

export interface ConvMessage {
  seq: number;
  role: 'system' | 'user' | 'assistant' | 'note' | string;
  kind: 'prompt' | 'message' | 'reply' | 'feedback' | 'refusal' | 'run_result' | 'rebuilt' | 'error' | string;
  content: string;
  created_at: string;
}

export interface Totals {
  conversations: number;
  messages: number;
  bytes: number;
}

export interface ConversationPage {
  items: Conversation[];
  total: number;
  page: number;
  page_size: number;
  totals: { archived: Totals; all: Totals };
}

export interface PrunePreview extends Totals {
  archived_before: string | null;
}

export interface PruneEntry {
  id: number;
  at: string;
  action: string;
  actor_name: string;
  as_user_id: string | null;
  detail: { archived_before?: string; conversations?: number; messages?: number; bytes?: number; error?: string };
}

const json = (body: unknown = {}): RequestInit => ({ method: 'POST', body: JSON.stringify(body) });

function qs(f: Record<string, string | number | undefined>): string {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v !== undefined && v !== '') u.set(k, String(v));
  const s = u.toString();
  return s ? `?${s}` : '';
}

export const listConversations = (f: Record<string, string | number | undefined>, signal?: AbortSignal) =>
  api<ConversationPage>(`/api/conversations${qs(f)}`, { signal });

export const getConversation = (id: number, sinceSeq = 0, signal?: AbortSignal) =>
  api<{ conversation: Conversation; messages: ConvMessage[] }>(`/api/conversations/${id}${qs({ since_seq: sinceSeq || undefined })}`, { signal });

export const getJobConversation = (jobId: string, sinceSeq = 0, signal?: AbortSignal) =>
  api<{ conversation: Conversation | null; messages: ConvMessage[]; restricted?: boolean }>(
    `/api/jobs/${encodeURIComponent(jobId)}/conversation${qs({ since_seq: sinceSeq || undefined })}`,
    { signal },
  );

export const archiveConversation = (id: number) => api<Conversation>(`/api/conversations/${id}/archive`, json());
export const unarchiveConversation = (id: number) => api<Conversation>(`/api/conversations/${id}/unarchive`, json());

export const getPrunePreview = (signal?: AbortSignal) => api<PrunePreview>('/api/conversations/prune', { signal });
export const pruneArchived = (archivedBefore: string) => api<Totals>('/api/conversations/prune', json({ confirm: 'prune', archived_before: archivedBefore }));
export const listPrunes = (signal?: AbortSignal) => api<{ items: PruneEntry[] }>('/api/conversations/prunes', { signal });

/** Mirrors the backend: canVerify roles read messages and archive. */
export const canReadMessages = (u: AuthUser | null) => !!u && ['Admin', 'DevOps', 'Developer', 'QA', 'QC'].includes(u.role);

/** Prune is for an Admin acting as itself (never while impersonating). */
export const canPrune = (u: AuthUser | null) => !!u && u.role === 'Admin' && !u.impersonator;

export const conversationHref = (id: number) => `/conversations/${id}`;

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export const KIND_LABEL: Record<string, string> = { job: 'Build', sync: 'Knowledge sync', call: 'Single call' };

/** Where a conversation's owner lives in the dashboard (null when it has no page). */
export function ownerHref(c: Pick<Conversation, 'kind' | 'ref' | 'product_id'>): string | null {
  if (c.kind === 'job') return `/jobs/${encodeURIComponent(c.ref)}`;
  if (c.kind === 'sync' && c.product_id) return `/products/${c.product_id}/knowledge?tab=syncs`;
  return null;
}
