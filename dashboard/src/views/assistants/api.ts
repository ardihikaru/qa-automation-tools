// Agents, skills, rules, commands and instructions synced from each product's automation repo
// (docs/designs/PRODUCT_KNOWLEDGE_SYNC.md "Amendment 2"). Shapes mirror
// ai-backend/internal/services/knowledge/query.go (Assistants, Entity) and api/knowledge.go
// (handleAssistants); the server stays the authority.
import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../products/api';
import type { Annotation } from '../knowledge/api';

export type AssistantKind = 'agent' | 'skill' | 'rule' | 'command' | 'instructions';

/** Display order, mirrors knowledge.AssistantKinds. */
export const ASSISTANT_KINDS: AssistantKind[] = ['agent', 'skill', 'rule', 'command', 'instructions'];

export const isAssistantKind = (v: string): v is AssistantKind => (ASSISTANT_KINDS as string[]).includes(v);

interface KindMeta {
  label: string;
  /** Lower-case noun for counts: "1 agent", "3 agents". */
  one: string;
  plural: string;
  /** Segment label on the rules page. */
  segment: string;
  /** Where the files live in the automation repo (mirrors knowledge.assistantKind). */
  where: string;
  /** List page that shows this kind. */
  listPath: string;
}

export const KIND_META: Record<AssistantKind, KindMeta> = {
  agent: { label: 'Agent', one: 'agent', segment: 'Agents', plural: 'agents', where: '.claude/agents/<name>.md', listPath: '/agents' },
  skill: { label: 'Skill', one: 'skill', segment: 'Skills', plural: 'skills', where: '.claude/skills/<name>/SKILL.md', listPath: '/skills?view=repo' },
  rule: { label: 'Rule', one: 'rule', segment: 'Rules', plural: 'rules', where: '.claude/rules/<name>.md', listPath: '/rules?kind=rule' },
  command: { label: 'Command', one: 'command', segment: 'Commands', plural: 'commands', where: '.claude/commands/<name>.md', listPath: '/rules?kind=command' },
  instructions: { label: 'Instructions', one: 'instructions file', segment: 'Instructions', plural: 'instructions files', where: 'CLAUDE.md or AGENTS.md', listPath: '/rules?kind=instructions' },
};

/** Parsed YAML frontmatter. Values are whatever the YAML held. */
export type Frontmatter = Record<string, unknown>;

/** A row of GET /api/assistants (no body). */
export interface AssistantItem {
  type: 'assistant';
  code: string;
  kind: AssistantKind;
  name: string;
  description: string;
  path: string;
  frontmatter: Frontmatter | null;
  origin: 'sync' | 'local';
  removed_at: string | null;
  removed_sync_id: number | null;
  changed_sync_id: number | null;
  product_id: number;
  product_name: string;
  source_url: string | null;
}

export interface AssistantPage {
  items: AssistantItem[];
  total: number;
  page: number;
  page_size: number;
  /** Counts per kind for the same filter without the kind. */
  kinds: Record<AssistantKind, number>;
}

/** GET /api/products/{pid}/knowledge/assistants/{code}: the full file. */
export interface AssistantDetail extends Omit<AssistantItem, 'product_name'> {
  body: string;
  annotation: Annotation | null;
  source_file?: string | null;
}

export interface AssistantFilter {
  product_id?: number;
  kind?: string;
  q?: string;
  page?: number;
  page_size?: number;
}

function qs(params: Record<string, string | number | undefined>): string {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '' || v === 0) continue;
    u.set(k, String(v));
  }
  const s = u.toString();
  return s ? `?${s}` : '';
}

const enc = encodeURIComponent;

export const listAssistants = (f: AssistantFilter, signal?: AbortSignal) =>
  api<AssistantPage>(`/api/assistants${qs({ ...f })}`, { signal }).then((r) => ({
    ...r,
    items: Array.isArray(r.items) ? r.items : [],
    kinds: { agent: 0, skill: 0, rule: 0, command: 0, instructions: 0, ...((r.kinds ?? {}) as Partial<Record<AssistantKind, number>>) },
  }));

export const getAssistant = (pid: number, code: string, signal?: AbortSignal) =>
  api<AssistantDetail>(`/api/products/${pid}/knowledge/assistants/${enc(code)}`, { signal });

export const putAssistantAnnotation = (pid: number, code: string, note: string, tags: string[]) =>
  api<Annotation>(`/api/products/${pid}/knowledge/annotations/assistant/${enc(code)}`, {
    method: 'PUT',
    body: JSON.stringify({ note, tags }),
  });

export const deleteAssistantAnnotation = (pid: number, code: string) =>
  api<void>(`/api/products/${pid}/knowledge/annotations/assistant/${enc(code)}`, { method: 'DELETE' });

/** Link to the detail page of one file. */
export const detailPath = (pid: number, code: string) => `/assistants/${pid}/${enc(code)}`;

// ---- products (for the filter) ----

export interface ProductOption {
  id: number;
  name: string;
}

let productsPromise: Promise<ProductOption[]> | null = null;

function loadProducts(): Promise<ProductOption[]> {
  if (!productsPromise) {
    productsPromise = api<{ products?: { id: number; name: string }[] }>('/api/products')
      .then((r) => (r.products ?? []).map((p) => ({ id: p.id, name: p.name })).sort((a, b) => a.name.localeCompare(b.name)))
      .catch((e) => {
        productsPromise = null; // let the next mount retry
        throw e;
      });
  }
  return productsPromise;
}

/** Products for the filter, loaded once per page load. An error leaves the list empty. */
export function useProducts() {
  const [products, setProducts] = useState<ProductOption[]>([]);
  const [error, setError] = useState(false);
  useEffect(() => {
    let alive = true;
    loadProducts()
      .then((p) => alive && setProducts(p))
      .catch(() => alive && setError(true));
    return () => {
      alive = false;
    };
  }, []);
  return { products, error };
}

// ---- frontmatter helpers ----

/** A frontmatter value as a list of strings: arrays as-is, "a, b" strings split on commas. */
export function fmList(fm: Frontmatter | null | undefined, ...keys: string[]): string[] {
  if (!fm) return [];
  for (const k of keys) {
    const v = fm[k];
    if (v === undefined || v === null || v === '') continue;
    if (Array.isArray(v)) return v.map((x) => (typeof x === 'string' ? x : JSON.stringify(x))).filter(Boolean);
    if (typeof v === 'string') return v.split(',').map((s) => s.trim()).filter(Boolean);
    return [String(v)];
  }
  return [];
}

export function fmString(fm: Frontmatter | null | undefined, key: string): string {
  const v = fm?.[key];
  return typeof v === 'string' || typeof v === 'number' ? String(v) : '';
}

/** Reads `?product=`: a positive id, or 0 for every product. */
export function parseProductParam(v: string | null): number {
  const n = Number(v);
  return Number.isInteger(n) && n > 0 ? n : 0;
}

/** `?product=` as a number (0 = every product) plus a setter that resets the page. */
export function useProductParam() {
  const [params, setParams] = useSearchParams();
  const raw = params.get('product') ?? '';
  const set = (v: string) => {
    const next = new URLSearchParams(params);
    if (v) next.set('product', v);
    else next.delete('product');
    next.delete('page');
    setParams(next);
  };
  return { raw, id: parseProductParam(raw), set };
}
