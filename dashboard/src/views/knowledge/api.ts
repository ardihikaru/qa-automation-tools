// Types and fetchers for the product knowledge base
// (docs/designs/PRODUCT_KNOWLEDGE_SYNC.md, ai-backend/internal/api/knowledge.go).
// Shapes mirror ai-backend/internal/services/knowledge/query.go; the server stays the authority.
import { ApiError, api } from '../products/api';
import { getKnowledgeLang } from './lang';

export { ApiError };

// ---- roles (mirror products.go: canVerify writes the knowledge base, canToggleWork sets the format) ----

export const canWriteKnowledge = (role: string) => ['Admin', 'DevOps', 'Developer', 'QA', 'QC'].includes(role);
export const canSetFormat = (role: string) => ['Admin', 'DevOps'].includes(role);

// ---- entity types ----

export type EntityType = 'part' | 'module' | 'submodule' | 'feature' | 'epic' | 'story' | 'test_case';

/** URL segment of each entity collection. */
export const TYPE_PATH: Record<EntityType, string> = {
  part: 'parts',
  module: 'modules',
  submodule: 'submodules',
  feature: 'features',
  epic: 'epics',
  story: 'stories',
  test_case: 'test-cases',
};

export const TYPE_LABEL: Record<EntityType, string> = {
  part: 'Part',
  module: 'Module',
  submodule: 'Sub-module',
  feature: 'Feature',
  epic: 'Epic',
  story: 'User story',
  test_case: 'Test case',
};

// ---- i18n (docs/designs/KNOWLEDGE_TRANSLATION.md); every shape assumption lives in this section ----

/** machine: LLM text; original: the author's own text; stale: older machine text; missing: not translated yet. */
export type I18nState = 'machine' | 'original' | 'stale' | 'missing';
export interface I18n {
  lang?: string;
  /** Item-level state (additive field of the read shape). */
  state?: I18nState;
  /** Per-field texts; when present they replace the English field values (see `localize`). */
  fields?: Record<string, { text?: string; state?: I18nState }>;
}

/** The state to show for an item: the item's own, else the weakest of its fields. */
export function i18nState(i: I18n | null | undefined): I18nState | undefined {
  if (!i) return undefined;
  if (i.state) return i.state;
  const st = Object.values(i.fields ?? {}).map((f) => f.state);
  for (const s of ['stale', 'machine'] as const) if (st.includes(s)) return s;
  return st.includes('original') ? 'original' : undefined;
}
/** Only machine and stale texts carry the "machine translated" marker. */
export const isMachine = (i: I18n | null | undefined) => {
  const s = i18nState(i);
  return s === 'machine' || s === 'stale';
};

/**
 * Applies `i18n.fields[f].text` over the English field `f` (an empty or missing text keeps the
 * English). Walks the whole response, so nested refs and children are covered too. Responses
 * whose English fields are already translated in place pass through untouched.
 */
export function localize<T>(v: T): T {
  if (Array.isArray(v)) {
    v.forEach(localize);
  } else if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>;
    const fields = (o.i18n as I18n | undefined)?.fields;
    if (fields) {
      for (const [f, x] of Object.entries(fields)) if (x?.text && x.state !== 'missing') o[f] = x.text;
    }
    Object.values(o).forEach(localize);
  }
  return v;
}

/** GET of the knowledge base in the user's language (`?lang=`). */
function kget<T>(path: string, init?: RequestInit): Promise<T> {
  return api<T>(`${path}${path.includes('?') ? '&' : '?'}lang=${getKnowledgeLang()}`, init).then(localize);
}

export const isEntityType = (v: string): v is EntityType => v in TYPE_PATH;

/** URL segment back to type (route `/products/:id/knowledge/:type/:code`). */
export function typeFromPath(seg: string | undefined): EntityType | null {
  const hit = (Object.entries(TYPE_PATH) as [EntityType, string][]).find(([, p]) => p === seg);
  return hit ? hit[0] : null;
}

/** Dashboard route of an entity's detail page. */
export const entityHref = (pid: number, type: EntityType, code: string) => `/products/${pid}/knowledge/${TYPE_PATH[type]}/${encodeURIComponent(code)}`;
export const knowledgeHref = (pid: number, tab?: string) => `/products/${pid}/knowledge${tab ? `?tab=${tab}` : ''}`;

/** Local draft codes (mirrors knowledge.LocalCodeRe). */
export const LOCAL_CODE_RE = /^L-[A-Z0-9-]{1,30}$/;
/** Any code accepted in an API path (mirrors knowledge.CodeRe). */
export const CODE_RE = /^[A-Za-z0-9.~_-]{1,64}$/;

export type Origin = 'sync' | 'local';

export interface Annotation {
  note: string | null;
  tags: string[] | null;
  updated_at: string;
}

/** Fields every entity item carries. */
export interface EntityBase {
  type: EntityType;
  code: string;
  origin: Origin;
  removed_at: string | null;
  removed_sync_id: number | null;
  changed_sync_id: number | null;
  first_sync_id: number | null;
  source_file: string | null;
  source_line: number | null;
  source_url: string | null;
  annotation: Annotation | null;
  i18n?: I18n;
}

/** A slim reference to a related entity (`refs` in query.go). `name` is the title, text or scenario. */
export interface Ref {
  code: string;
  name: string;
  origin: Origin;
  removed_at: string | null;
  i18n?: I18n;
}

export interface Part extends EntityBase {
  name: string;
}
export interface Module extends EntityBase {
  part_code: string;
  name: string;
}
export interface Submodule extends EntityBase {
  module_code: string;
  name: string;
}
export interface Feature extends EntityBase {
  module_code: string;
  submodule_code: string | null;
  name: string;
  repo_entry: string;
  release_status: string;
  status_as_of: string;
  /** Raw catalogue "Prod since" (YYYY-MM, YYYY-MM..YYYY-MM, <YYYY-MM, ?); '' when absent or not PROD. */
  prod_since?: string;
  /** Raw catalogue "Periode pelaksanaan" (development period: YYYY-MM or YYYY-MM..YYYY-MM); '' when unknown. */
  dev_period?: string;
  evidence: string;
  notes: string;
}
export interface Epic extends EntityBase {
  title: string;
  service: string;
  area: string;
  /** Only in the epics list. */
  case_counts?: Record<string, number>;
  test_cases?: number;
  /** CASE_VERIFICATION_AND_EPIC_TAB.md: human verification and last run counts (epics list only). */
  verified_valid?: number;
  verified_invalid?: number;
  last_pass?: number;
  last_fail?: number;
  /** Present when the server sends the shared coverage shape for the epic. */
  coverage?: Coverage;
}

export type Verdict = 'valid' | 'invalid';
export interface Verification {
  verdict: Verdict;
  verified_by: string;
  verified_by_name: string;
  verified_at: string;
  note: string;
}
export interface Story extends EntityBase {
  epic_code: string;
  text: string;
  unmapped_reason: string | null;
}
export interface TestCase extends EntityBase {
  epic_code: string;
  story_code: string | null;
  scenario: string;
  polarity: string;
  level: string;
  priority: string;
  status: string;
  automated: boolean;
  spec_paths: string[] | null;
  reason_category: string;
  reason_detail: string;
  /** Latest human verification, null when nobody verified it. */
  verification?: Verification | null;
  /** Optional: how many times it was verified. Shown only when the server sends it. */
  verification_count?: number;
}

export interface FeatureStats {
  stories: number;
  test_cases: number;
  automated: number;
  coverage_pct: number;
}

// ---- coverage (mirrors knowledge.Coverage in coverage.go) ----

/** untested: no test cases; none: none automated; partial; full: every case automated. */
export type CoverageState = 'untested' | 'none' | 'partial' | 'full';
export const COVERAGE_STATES: CoverageState[] = ['untested', 'none', 'partial', 'full'];

/** Distinct active test cases under an entity, by status. */
export interface Coverage {
  test_cases: number;
  automated: number;
  planned: number;
  blocked: number;
  manual_only: number;
  wont: number;
  pct: number;
  state: CoverageState;
}

export const EMPTY_COVERAGE: Coverage = { test_cases: 0, automated: 0, planned: 0, blocked: 0, manual_only: 0, wont: 0, pct: 0, state: 'untested' };

export type CoverageBy = 'part' | 'module' | 'submodule' | 'feature' | 'epic' | 'story';

type WithCov<T> = T & { coverage?: Coverage };

// Detail shapes (GET {type}/{code}). Single parents come back as 0- or 1-item arrays.
// `coverage` is the entity's own; listed children carry theirs (query.go entityCoverage).
export type PartDetail = WithCov<Part> & { modules: WithCov<Ref>[] };
export type ModuleDetail = WithCov<Module> & { part: Ref[]; submodules: WithCov<Ref>[]; features: WithCov<Feature>[] };
export type SubmoduleDetail = WithCov<Submodule> & { module: Ref[]; features: WithCov<Feature>[] };
/** One piece of evidence of a feature row, parsed from its Evidence cell (Amendment 3). */
export interface EvidenceItem {
  kind: string; // GIT, SRV, DOC, RN, S, BRD or TEXT
  raw: string;
  label: string;
  repo?: string;
  sha?: string;
  date?: string;
  url?: string;
}
/** A sync as referenced from an entity: when it finished and on which catalogue commit. */
export interface SyncRef {
  id: number;
  finished_at?: string;
  commit_sha?: string;
}
export type FeatureDetail = WithCov<Feature> & {
  module: Ref[];
  submodule: Ref[];
  stories: (WithCov<Story> & { test_cases: TestCase[] })[];
  stats: FeatureStats;
  evidence_items?: EvidenceItem[];
  /** The sync that last changed this row (its catalogue data); first_sync = when it appeared. */
  changed_sync?: SyncRef | null;
  first_sync?: SyncRef | null;
};
export type EpicDetail = WithCov<Epic> & { stories: WithCov<Story>[]; test_cases: Ref[] };
export type StoryDetail = WithCov<Story> & { epic: Ref[]; features: WithCov<Ref>[]; test_cases: TestCase[]; feature_codes: string[] };
export type TestCaseDetail = TestCase & { epic: Ref[]; story: Ref[]; features: WithCov<Ref>[] };

/** GET knowledge/modules item. */
export type ModuleItem = Module & { coverage: Coverage; features: number; submodules: number };
/** GET knowledge/features item. */
export type FeatureItem = Feature & { coverage: Coverage };
/** GET knowledge/stories item. */
export type StoryItem = Story & { coverage?: Coverage };

export type EntityDetail = PartDetail | ModuleDetail | SubmoduleDetail | FeatureDetail | EpicDetail | StoryDetail | TestCaseDetail;

// ---- syncs ----

export type SyncStatus = 'running' | 'succeeded' | 'failed' | 'skipped';
export type AiStatus = 'none' | 'pending' | 'done' | 'failed';
export type DiffKind = 'added' | 'updated' | 'removed' | 'restored';

export interface Counts {
  parts: number;
  modules: number;
  submodules: number;
  features: number;
  features_with_stories: number;
  epics: number;
  stories: number;
  stories_unmapped: number;
  test_cases: number;
  automated: number;
}

export interface Diff {
  unchanged: boolean;
  total: Record<DiffKind, number>;
  /** Only on GET syncs/{id}. Keys are entity types. */
  by_type?: Partial<Record<EntityType, Partial<Record<DiffKind, string[]>>>>;
  truncated: boolean;
}

export interface ProblemItem {
  file: string;
  line: number;
  msg: string;
}
export interface Problems {
  total: number;
  by_message: Record<string, number>;
  /** Only on GET syncs/{id}; capped at 500. */
  items?: ProblemItem[];
}

export interface SyncRow {
  id: number;
  status: SyncStatus;
  trigger_kind: 'manual' | 'schedule';
  triggered_by: string | null;
  repo_id: number | null;
  repo_name: string;
  repo_url: string;
  branch: string;
  format: string;
  parser_version: number;
  commit_sha: string | null;
  started_at: string;
  finished_at: string | null;
  counts: Counts | null;
  diff: Diff | null;
  problems: Problems | null;
  ai_status: AiStatus;
  ai_summary: string | null;
  ai_error: string | null;
  error: string | null;
}

export type Enums = Record<'release' | 'polarity' | 'level' | 'priority' | 'status' | 'reason_category', string[]>;

export interface Summary {
  format: string;
  formats: string[];
  configured: boolean;
  enums: Enums;
  repo: { id: number; name: string; url: string; branch: string } | null;
  latest_sync: SyncRow | null;
  last_success: SyncRow | null;
  counts: Counts;
  coverage: { test_cases: number; automated: number; pct: number };
  local: { stories: number; test_cases: number };
}

// ---- tree ----

export interface TreeFeature {
  code: string;
  name: string;
  release_status: string;
  status_as_of: string;
  /** Raw catalogue "Prod since" (YYYY-MM, YYYY-MM..YYYY-MM, <YYYY-MM, ?); '' when absent or not PROD. */
  prod_since?: string;
  /** Raw catalogue "Periode pelaksanaan" (development period: YYYY-MM or YYYY-MM..YYYY-MM); '' when unknown. */
  dev_period?: string;
  origin: Origin;
  removed_at: string | null;
  stories: number;
  test_cases: number;
  automated: number;
  coverage_pct: number;
  has_annotation: boolean;
  i18n?: I18n;
}
interface TreeNode {
  code: string;
  name: string;
  origin: Origin;
  removed_at: string | null;
  has_annotation: boolean;
  i18n?: I18n;
}
export interface TreeSubmodule extends TreeNode {
  features: TreeFeature[];
}
export interface TreeModule extends TreeNode {
  submodules: TreeSubmodule[];
  features_without_submodule: TreeFeature[];
}
export interface TreePart extends TreeNode {
  modules: TreeModule[];
}
export interface Tree {
  parts: TreePart[];
}

export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

// ---- request bodies ----

export interface StoryDraft {
  /** Language the text was typed in (a hint; the server detects it). */
  lang?: 'en' | 'id';
  code?: string;
  epic_code: string;
  text: string;
  feature_codes: string[];
}
export interface TestCaseDraft {
  lang?: 'en' | 'id';
  code?: string;
  epic_code: string;
  story_code: string;
  scenario: string;
  polarity: string;
  level: string;
  priority: string;
  status: string;
  spec_paths: string[];
  reason_category: string;
  reason_detail: string;
}

/** Test case statuses that must state a reason (mirrors knowledge.NeedsReason). */
export const NEEDS_REASON = ['blocked', 'manual-only', 'wont'];

// ---- fetchers ----

const enc = encodeURIComponent;
const kb = (pid: number) => `/api/products/${pid}/knowledge`;

function qs(params: Record<string, string | number | boolean | undefined>): string {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '' || v === false) continue;
    u.set(k, v === true ? '1' : String(v));
  }
  const s = u.toString();
  return s ? `?${s}` : '';
}

const json = (method: string, body: unknown, signal?: AbortSignal): RequestInit => ({ method, body: JSON.stringify(body), signal });

export const getSummary = (pid: number, signal?: AbortSignal) => api<Summary>(`${kb(pid)}/summary`, { signal });

export const getTree = (pid: number, includeRemoved: boolean, signal?: AbortSignal) =>
  kget<Tree>(`${kb(pid)}/tree${qs({ include_removed: includeRemoved })}`, { signal });

export const listSyncs = (pid: number, limit = 50, signal?: AbortSignal) =>
  api<{ syncs: SyncRow[] }>(`${kb(pid)}/syncs${qs({ limit })}`, { signal }).then((r) => r.syncs ?? []);

export const getSync = (pid: number, sid: number, signal?: AbortSignal) => api<SyncRow>(`${kb(pid)}/syncs/${sid}`, { signal });

/** Starts a sync. A 409 for a running sync is returned as `{sync_id, attached: true}` so the caller can follow it. */
export async function startSync(pid: number): Promise<{ sync_id: number; attached: boolean }> {
  try {
    const r = await api<{ sync_id: number }>(`${kb(pid)}/sync`, { method: 'POST' });
    return { sync_id: r.sync_id, attached: false };
  } catch (e) {
    const sid = e instanceof ApiError && e.status === 409 ? Number(e.body.sync_id) : NaN;
    if (Number.isFinite(sid) && sid > 0) return { sync_id: sid, attached: true };
    throw e;
  }
}

export const listEpics = (pid: number, includeRemoved = false, signal?: AbortSignal) =>
  kget<{ epics: Epic[] }>(`${kb(pid)}/epics${qs({ include_removed: includeRemoved })}`, { signal }).then((r) => r.epics ?? []);

export interface ScopeFilter {
  part?: string;
  module?: string;
  submodule?: string;
}

export interface StoryFilter extends ScopeFilter {
  epic?: string;
  feature?: string;
  /** Comma-separated coverage states. */
  coverage?: string;
  origin?: string;
  tag?: string;
  q?: string;
  include_removed?: boolean;
  page?: number;
  page_size?: number;
}
export const listStories = (pid: number, f: StoryFilter, signal?: AbortSignal) =>
  kget<Page<StoryItem>>(`${kb(pid)}/stories${qs({ ...f })}`, { signal });

export interface TestCaseFilter extends StoryFilter {
  story?: string;
  status?: string;
  level?: string;
  priority?: string;
  /** "1" or "0" */
  automated?: string;
  /** any | valid | invalid | none */
  verified?: string;
  /** User id of the verifier (implies any). */
  verified_by?: string;
}
export const listTestCases = (pid: number, f: TestCaseFilter, signal?: AbortSignal) =>
  kget<Page<TestCase>>(`${kb(pid)}/test-cases${qs({ ...f })}`, { signal });

export interface FeatureFilter extends ScopeFilter {
  release?: string;
  /** "tested" (server default: features with test cases first) or "code". */
  sort?: 'tested' | 'code';
  /** Comma-separated coverage states. */
  coverage?: string;
  q?: string;
  origin?: string;
  tag?: string;
  include_removed?: boolean;
  page?: number;
  page_size?: number;
}
export const listFeatures = (pid: number, f: FeatureFilter, signal?: AbortSignal) =>
  kget<Page<FeatureItem>>(`${kb(pid)}/features${qs({ ...f })}`, { signal });

export const listModules = (pid: number, includeRemoved = false, signal?: AbortSignal) =>
  kget<{ modules: ModuleItem[] }>(`${kb(pid)}/modules${qs({ include_removed: includeRemoved })}`, { signal }).then((r) => r.modules ?? []);

export const getCoverage = (pid: number, by: CoverageBy, scope: ScopeFilter & { epic?: string } = {}, signal?: AbortSignal) =>
  api<{ by: string; coverage: Record<string, Coverage> | null }>(`${kb(pid)}/coverage${qs({ by, ...scope })}`, { signal }).then((r) => r.coverage ?? {});

/** The server caps page_size at 200 (query.go Filter.norm). */
export const MAX_PAGE_SIZE = 200;

/** Every active test case matching `f`, fetched page by page (first page, then the rest in parallel). */
export async function listAllTestCases(pid: number, f: TestCaseFilter, signal?: AbortSignal): Promise<TestCase[]> {
  const first = await listTestCases(pid, { ...f, page: 1, page_size: MAX_PAGE_SIZE }, signal);
  const pages = Math.ceil(first.total / MAX_PAGE_SIZE);
  if (pages <= 1) return first.items ?? [];
  const rest = await Promise.all(Array.from({ length: pages - 1 }, (_, i) => listTestCases(pid, { ...f, page: i + 2, page_size: MAX_PAGE_SIZE }, signal)));
  return [...(first.items ?? []), ...rest.flatMap((p) => p.items ?? [])];
}

export const getEntity = <T extends EntityDetail = EntityDetail>(pid: number, type: EntityType, code: string, signal?: AbortSignal) =>
  kget<T>(`${kb(pid)}/${TYPE_PATH[type]}/${enc(code)}`, { signal });

export const putAnnotation = (pid: number, type: EntityType, code: string, note: string, tags: string[]) =>
  api<Annotation>(`${kb(pid)}/annotations/${type}/${enc(code)}`, json('PUT', { note, tags }));

export const deleteAnnotation = (pid: number, type: EntityType, code: string) =>
  api<void>(`${kb(pid)}/annotations/${type}/${enc(code)}`, { method: 'DELETE' });

type DraftType = 'story' | 'test_case';

/** Creates (no `code` argument) or updates a local draft. */
export function saveDraft(pid: number, type: DraftType, body: StoryDraft | TestCaseDraft, code?: string) {
  const base = `${kb(pid)}/${TYPE_PATH[type]}`;
  if (code === undefined) return api<EntityDetail>(base, json('POST', body));
  const rest = { ...body };
  delete rest.code;
  return api<EntityDetail>(`${base}/${enc(code)}`, json('PUT', rest));
}

export const deleteDraft = (pid: number, type: DraftType, code: string) =>
  api<void>(`${kb(pid)}/${TYPE_PATH[type]}/${enc(code)}`, { method: 'DELETE' });

// ---- formatting helpers ----

/** Parses a backend timestamp (RFC 3339; a bare "YYYY-MM-DD hh:mm:ss" is read as UTC). */
export function parseTime(s: string | null | undefined): Date | null {
  if (!s) return null;
  let v = s.trim();
  if (/^\d{4}-\d{2}-\d{2} \d/.test(v)) v = v.replace(' ', 'T');
  if (/T\d{2}:\d{2}(:\d{2}(\.\d+)?)?$/.test(v)) v += 'Z';
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? null : d;
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });

export function relativeTime(s: string | null | undefined, now = Date.now()): string {
  const d = parseTime(s);
  if (!d) return '';
  const sec = Math.round((d.getTime() - now) / 1000);
  const abs = Math.abs(sec);
  if (abs < 45) return 'just now';
  if (abs < 3600) return rtf.format(Math.round(sec / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(sec / 3600), 'hour');
  if (abs < 86400 * 30) return rtf.format(Math.round(sec / 86400), 'day');
  return d.toLocaleDateString();
}

export const absoluteTime = (s: string | null | undefined) => parseTime(s)?.toLocaleString() ?? '';

export function duration(from: string | null | undefined, to: string | null | undefined): string {
  const a = parseTime(from);
  const b = parseTime(to);
  if (!a || !b) return '';
  return formatMs(b.getTime() - a.getTime());
}

/** A duration in milliseconds as "850ms", "42s", "3m 5s" or "2h 10m" ('' for null/undefined). */
export function formatMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || !Number.isFinite(ms)) return '';
  if (ms > 0 && ms < 1000) return `${Math.round(ms)}ms`;
  const sec = Math.max(0, Math.round(ms / 1000));
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  return m < 60 ? `${m}m ${sec % 60}s` : `${Math.floor(m / 60)}h ${m % 60}m`;
}

export const shortSha = (sha: string | null | undefined) => (sha ? sha.slice(0, 7) : '');

/** `owner/repo` of a GitHub URL (https or ssh form), or null for other hosts. */
export function githubSlug(url: string | null | undefined): string | null {
  if (!url) return null;
  const m = url.trim().match(/github\.com[/:]([^/\s]+\/[^/\s]+?)(?:\.git)?\/*$/i);
  return m ? m[1] : null;
}

export function commitUrl(repoUrl: string | null | undefined, sha: string | null | undefined): string | null {
  const slug = githubSlug(repoUrl);
  return slug && sha ? `https://github.com/${slug}/commit/${sha}` : null;
}

export function errorText(e: unknown, fallback = 'Request failed'): string {
  if (e instanceof ApiError) {
    if (e.status === 503) return `${e.message} (the knowledge base service is unavailable)`;
    return e.message;
  }
  return e instanceof Error ? e.message : fallback;
}

export const isAbort = (e: unknown) => e instanceof DOMException && e.name === 'AbortError';

export const pct = (n: number) => `${Math.round(n)}%`;

// ---- case automation (docs/designs/CASE_AUTOMATION.md section 5) ----

export type JobStatus = 'queued' | 'pending' | 'running' | 'completed' | 'failed' | string;
export const isTerminalJob = (s: string | undefined) => s === 'completed' || s === 'failed';

export interface JobEvent {
  seq: number;
  at?: string;
  level?: string;
  step?: string;
  message?: string;
}
export type ApprovalDecision = 'approve' | 'reject' | 'changes';
export interface JobApproval {
  status?: string;
  gaos_created?: boolean;
  answer_text?: string;
  answered_by?: string;
  /** HISTORY_ANSWER.md: where the answer came from and who gave it. */
  source?: string;
  actor?: string;
  answered_at?: string | null;
}
export interface JobReport {
  passed?: number;
  failed?: number;
  skipped?: number;
}
/** Answer of GET /api/jobs/{id}/events. Everything but `events` may be missing. */
export interface JobEvents {
  events: JobEvent[];
  status?: JobStatus;
  phase?: string;
  approval?: JobApproval | null;
  pr_url?: string;
  // Optional extras the backend may add; the view degrades without them.
  case_code?: string;
  branch?: string;
  type?: string;
  /** The job type (case_run, case_create), sent by GET /api/jobs/{id}/events. */
  job_type?: string;
  product_id?: number;
  spec?: { path?: string; content?: string } | null;
  report?: JobReport | null;
  /** case_run verdict (pass|fail|skipped|none) and the skip reason; a skipped run is status completed. */
  outcome?: string;
  skip_reason?: string;
  /** drafts_publish jobs: local draft code -> repo code (assumed to be echoed by the events endpoint). */
  codes?: Record<string, string> | null;
}
/** One row of GET .../test-cases/{code}/jobs. */
export interface CaseJob {
  id: number | string;
  type?: string;
  status: JobStatus;
  phase?: string;
  branch?: string;
  pr_url?: string;
  created_at?: string;
  outcome?: string;
}

const caseUrl = (pid: number, code: string) => `${kb(pid)}/test-cases/${enc(code)}`;

export const runCase = (pid: number, code: string) => api<{ job_id: number | string }>(`${caseUrl(pid, code)}/run`, { method: 'POST' });
export const createAutomation = (pid: number, code: string) => api<{ job_id: number | string }>(`${caseUrl(pid, code)}/create-automation`, { method: 'POST' });
export const listCaseJobs = (pid: number, code: string, signal?: AbortSignal) =>
  api<{ jobs?: CaseJob[] } | CaseJob[]>(`${caseUrl(pid, code)}/jobs`, { signal }).then((r) => (Array.isArray(r) ? r : (r?.jobs ?? [])));

/** GET /api/jobs/{id}: timing fields are RFC3339 UTC strings or null. */
export interface JobInfo {
  id?: number | string;
  status?: JobStatus;
  case_code?: string | null;
  product_id?: number | null;
  created_at?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  duration_ms?: number | null;
  elapsed_ms?: number | null;
}
export const getJob = (jobId: number | string, signal?: AbortSignal) => api<JobInfo>(`/api/jobs/${enc(String(jobId))}`, { signal });

export interface CaseJobRow {
  id: number | string;
  type?: string;
  status: JobStatus;
  error?: string | null;
  created_at?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  pr_url?: string | null;
  branch?: string | null;
  outcome?: string | null;
}
export interface CaseJobHistory {
  jobs: CaseJobRow[];
  summary: { total: number; success: number; failed: number };
}
export const listCaseJobHistory = (pid: number, code: string, limit = 10, signal?: AbortSignal) =>
  api<Partial<CaseJobHistory>>(`${caseUrl(pid, code)}/jobs${qs({ limit })}`, { signal }).then((r) => ({
    jobs: r?.jobs ?? [],
    summary: r?.summary ?? { total: r?.jobs?.length ?? 0, success: 0, failed: 0 },
  }));

// ---- case verification ----
export interface Verifier {
  id: string;
  name: string;
  count: number;
}
export const listVerifiers = (pid: number, signal?: AbortSignal) =>
  api<{ verifiers?: Verifier[] }>(`${kb(pid)}/test-cases/verifiers`, { signal }).then((r) => r?.verifiers ?? []);
export const putVerification = (pid: number, code: string, verdict: Verdict | null, note?: string) =>
  api<Verification | null>(`${caseUrl(pid, code)}/verification`, json('PUT', { verdict, ...(note ? { note } : {}) })).then((r) => r ?? null);

export interface VerificationEvent {
  id: number | string;
  verdict: Verdict | 'cleared';
  note?: string | null;
  verified_by?: string;
  verified_by_name?: string;
  verified_at: string;
}
export const listVerificationHistory = (pid: number, code: string, limit = 50, signal?: AbortSignal) =>
  api<{ history?: VerificationEvent[]; total?: number }>(`${caseUrl(pid, code)}/verification/history${qs({ limit })}`, { signal }).then((r) => ({
    history: r?.history ?? [],
    total: r?.total ?? r?.history?.length ?? 0,
  }));

export const getJobEvents = (jobId: number | string, since: number, signal?: AbortSignal) =>
  api<JobEvents>(`/api/jobs/${enc(String(jobId))}/events?since=${since}`, { signal }).then((r) => ({ ...r, events: Array.isArray(r?.events) ? r.events : [] }));
export const answerApproval = (jobId: number | string, decision: ApprovalDecision, text?: string) =>
  api<unknown>(`/api/jobs/${enc(String(jobId))}/approval`, json('POST', { decision, ...(text ? { text } : {}) }));

// ---- EPIC_AUTOMATION.md: automate an epic's planned cases, two at a time ----
export type EpicCaseState = 'pending' | 'active' | 'awaiting_approval' | 'pr' | 'declined' | 'failed' | 'needs_person' | 'skipped';
export interface EpicBatchCase {
  code: string;
  seq: number;
  state: EpicCaseState;
  job_id: string;
  detail: string;
}
export interface EpicBatch {
  id: string;
  status: 'running' | 'completed' | 'cancelled' | 'stopped';
  note: string;
  started_by: string;
  created_at: string;
  max_open: number;
  cases: EpicBatchCase[];
}
export interface EpicAutomation {
  repo: boolean;
  eligible: number;
  sample: string[];
  batch: EpicBatch | null;
}

const epicUrl = (pid: number, epic: string) => `${kb(pid)}/epics/${enc(epic)}`;
export const getEpicAutomation = (pid: number, epic: string, signal?: AbortSignal) => api<EpicAutomation>(`${epicUrl(pid, epic)}/automation`, { signal });
export const automateEpic = (pid: number, epic: string, maxCases: number) =>
  api<{ batch_id: string; planned: string[] }>(`${epicUrl(pid, epic)}/automate`, json('POST', { max_cases: maxCases }));
export const cancelEpicBatch = (id: string) => api<EpicBatch>(`/api/epic-batches/${enc(id)}/cancel`, { method: 'POST' });

// ---- module checklist export (docs/designs/MODULE_CHECKLIST_EXPORT.md) ----

export type ChecklistFormat = 'pdf' | 'xlsx' | 'csv';
/** all = every feature; prod = PROD features only, optionally those whose "Prod since" overlaps [sinceFrom, sinceTo]. */
export type ChecklistScope = 'all' | 'prod';
export type ChecklistCache = 'hit' | 'miss' | 'regenerated' | 'bypass';

export interface ChecklistOptions {
  format: ChecklistFormat;
  scope?: ChecklistScope;
  /** YYYY-MM, only with scope 'prod'; empty = open. */
  sinceFrom?: string;
  sinceTo?: string;
  /** 'unknown': only PROD features without a known Prod sejak (scope 'prod', no period). */
  prodSince?: 'unknown';
  /** false leaves out the Cek (manual tick box) column; default true. */
  checkColumn?: boolean;
  /** true adds the "Periode pelaksanaan" (development period) column. */
  devPeriod?: boolean;
  lastUpdate: boolean;
  force: boolean;
}

/**
 * Downloads the modules checklist and saves it through a temporary object URL.
 * Resolves with the `X-Export-Cache` value, or null when the header is unreadable (cross-origin
 * local development, where Content-Disposition is unreadable too and a generated name is used).
 */
export async function downloadChecklist(pid: number, opts: ChecklistOptions, signal?: AbortSignal): Promise<ChecklistCache | null> {
  const qs = new URLSearchParams({ format: opts.format, last_update: opts.lastUpdate ? '1' : '0', force: opts.force ? '1' : '0' });
  if (opts.scope === 'prod') {
    qs.set('scope', 'prod');
    if (opts.sinceFrom) qs.set('since_from', opts.sinceFrom);
    if (opts.sinceTo) qs.set('since_to', opts.sinceTo);
    if (opts.prodSince === 'unknown') qs.set('prod_since', 'unknown');
  }
  if (opts.checkColumn === false) qs.set('check_column', '0');
  if (opts.devPeriod) qs.set('dev_period', '1');
  const res = await fetch(`/api/products/${pid}/knowledge/exports/modules-checklist?${qs}`, { credentials: 'include', signal });
  if (!res.ok) {
    let body: Record<string, unknown> = {};
    try {
      body = await res.json();
    } catch {
      // not JSON
    }
    const msg = typeof body.error === 'string' && body.error ? body.error : `Request failed (${res.status})`;
    throw new ApiError(msg, res.status, body);
  }
  const blob = await res.blob();
  const cd = res.headers.get('Content-Disposition') ?? '';
  const name = /filename="([^"\r\n]+)"/.exec(cd)?.[1] ?? `product-${pid}-MODULE-CHECKLIST.${opts.format}`;
  const url = URL.createObjectURL(blob);
  try {
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    a.style.display = 'none';
    document.body.appendChild(a);
    a.click();
    a.remove();
  } finally {
    // Revoke after the browser has started the save.
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
  }
  const cache = res.headers.get('X-Export-Cache');
  return cache === 'hit' || cache === 'miss' || cache === 'regenerated' || cache === 'bypass' ? cache : null;
}

const ID_MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];
const MONTH_RE = /^(\d{4})-(0[1-9]|1[0-2])$/;

function monthText(m: string): string {
  const x = MONTH_RE.exec(m);
  return x ? `${ID_MONTHS[Number(x[2]) - 1]} ${x[1]}` : m;
}

/** "2024-07..2025-04" -> "Jul 2024 – Apr 2025", "2024-07..2024-12" -> "Jul – Des 2024", "2024-09" -> "Sep 2024"; '' stays ''. */
export function periodText(raw?: string | null): string {
  if (!raw) return '';
  const [a, b] = raw.split('..');
  if (!b || a === b) return monthText(a);
  if (a.slice(0, 4) === b.slice(0, 4)) return `${monthText(a).split(' ')[0]} – ${monthText(b)}`;
  return `${monthText(a)} – ${monthText(b)}`;
}

/** Development period of a module or sub-module: earliest start to latest end of its features ('' when none has one). */
export function devPeriodRollup(features: { dev_period?: string | null }[]): string {
  let from = '';
  let to = '';
  for (const f of features) {
    const raw = f.dev_period;
    if (!raw) continue;
    const [a, b = a] = raw.split('..');
    if (!MONTH_RE.test(a) || !MONTH_RE.test(b)) continue;
    if (!from || a < from) from = a;
    if (b > to) to = b;
  }
  if (!from) return '';
  return from === to ? from : `${from}..${to}`;
}

// ---- translation status (admin chip) ----

export type TranslationRunState = 'idle' | 'queued' | 'running' | 'done' | 'failed' | string;
export interface TranslationStatus {
  state: TranslationRunState;
  total: number;
  done: number;
  stale: number;
  failed: number;
}
interface TranslationStatusWire {
  enabled?: boolean;
  running?: boolean;
  lang_coverage?: { total: number; translated: number; stale: number; missing: number; failed: number };
}
export const getTranslationStatus = (pid: number, signal?: AbortSignal): Promise<TranslationStatus> =>
  api<TranslationStatusWire>(`${kb(pid)}/translations/status`, { signal }).then((w) => {
    const c = w?.lang_coverage ?? { total: 0, translated: 0, stale: 0, missing: 0, failed: 0 };
    return { state: w?.running ? 'running' : 'idle', total: c.total, done: c.translated, stale: c.stale, failed: c.failed };
  });

// ---- publishing drafts to the automation repo as a PR ----

export type PublicationState = 'queued' | 'running' | 'pr_open' | 'merged' | 'failed';
export interface Publication {
  id: number | string;
  epic_code: string;
  state: PublicationState;
  pr_url?: string | null;
  branch?: string | null;
  job_id?: number | string | null;
  /** Local code -> repo code, e.g. {'L-ABC-1': 'TC-OUTREG-042'}; filled once published. */
  codes?: Record<string, string> | null;
}
export const isActivePublication = (p: Publication) => p.state === 'queued' || p.state === 'running';
export const listPublications = (pid: number, signal?: AbortSignal) =>
  api<{ publications?: Publication[] }>(`${kb(pid)}/publications`, { signal }).then((r) => r?.publications ?? []);
export const publishDrafts = (pid: number, epicCode: string) =>
  api<{ job_id: number | string }>(`${kb(pid)}/publish`, json('POST', { epic_code: epicCode }));
