export type FixTrackingStatus = 'open' | 'in_progress' | 'fixed' | 'verified';
export type FixTrackingSeverity = 'low' | 'medium' | 'high' | 'critical';
export type FixTrackingCategory = 'frontend' | 'backend' | 'unknown';

export interface Suggestion {
  id: number;
  suggestion: string;
  confidence?: number;
  target?: string;
  file_path?: string;
  diff?: string | null;
}

export interface FixTrackingIssueRecord {
  id: number;
  title: string;
  page: string;
  action: string;
  error_type: string;
  error_text: string;
  category: FixTrackingCategory;
  severity: FixTrackingSeverity;
  status: FixTrackingStatus;
  is_fixed: boolean;
  notes: string;
  created_at: string;
  resolved_at?: string;
  fix_commit_url?: string;
  fix_pr_url?: string;
  resolution_hours?: number;
  suggestions?: Suggestion[];
}

export interface FixTrackingAnalytics {
  totalIssues: number;
  fixedCount: number;
  verifiedCount: number;
  openCount: number;
  inProgressCount: number;
  fixRatePercentage: number;
  averageTimeToResolveHours: number;
  issuesBySeverity: Record<FixTrackingSeverity, number>;
}

export interface FixTrackingListResponse {
  data: FixTrackingIssueRecord[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    total_pages: number;
  };
  analytics: FixTrackingAnalytics;
}

interface FixTrackingMockApiOptions {
  count?: number;
  minLatencyMs?: number;
  maxLatencyMs?: number;
}

interface IssueQuery {
  page: number;
  limit: number;
  sortBy: 'date_newest' | 'date_oldest' | 'severity_high' | 'severity_low';
  isFixed?: boolean;
  category?: FixTrackingCategory;
  search?: string;
}

const PAGES = ['login', 'checkout', 'search', 'profile', 'settings', 'reports', 'billing', 'orders'] as const;
const ACTIONS = [
  'click_submit',
  'open_modal',
  'submit_form',
  'navigate_next',
  'toggle_filter',
  'upload_document',
  'create_order',
] as const;
const ERROR_TYPES = ['validation_error', 'timeout_error', 'network_error', 'ui_error', 'state_error'] as const;
const TITLE_PREFIX = ['Regression', 'Blocking', 'Flaky', 'Intermittent', 'Critical Path', 'Permission'] as const;
function randomInt(min: number, max: number): number {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function randomItem<T>(items: readonly T[]): T {
  return items[randomInt(0, items.length - 1)];
}

function shuffle<T>(items: T[]): T[] {
  for (let i = items.length - 1; i > 0; i -= 1) {
    const j = randomInt(0, i);
    [items[i], items[j]] = [items[j], items[i]];
  }
  return items;
}

function randomDateWithinLastDays(days: number): Date {
  const now = Date.now();
  const windowMs = days * 24 * 60 * 60 * 1000;
  return new Date(now - randomInt(5 * 60 * 1000, windowMs));
}

function pickSeverity(index: number): FixTrackingSeverity {
  if (index % 10 === 0) return 'critical';
  if (index % 4 === 0) return 'high';
  if (index % 2 === 0) return 'medium';
  return 'low';
}

function pickCategory(index: number): FixTrackingCategory {
  if (index % 6 === 0) return 'backend';
  if (index % 5 === 0) return 'unknown';
  return 'frontend';
}

function buildStatusPool(count: number): FixTrackingStatus[] {
  const fixedCount = Math.round(count * 0.6);
  const verifiedCount = Math.round(count * 0.2);
  const openCount = Math.round(count * 0.15);
  const inProgressCount = Math.max(0, count - fixedCount - verifiedCount - openCount);

  return shuffle([
    ...Array.from({ length: fixedCount }, () => 'fixed' as const),
    ...Array.from({ length: verifiedCount }, () => 'verified' as const),
    ...Array.from({ length: openCount }, () => 'open' as const),
    ...Array.from({ length: inProgressCount }, () => 'in_progress' as const),
  ]);
}

function buildSuggestions(issueId: number, category: FixTrackingCategory): Suggestion[] {
  const target = category === 'backend' ? 'api' : 'ui';
  return [
    {
      id: issueId * 10 + 1,
      suggestion: category === 'backend' ? 'Add server-side guard and return typed error payload.' : 'Add explicit client-side validation before submit.',
      confidence: Number((randomInt(72, 95) / 100).toFixed(2)),
      target,
      file_path: category === 'backend' ? 'internal/api/router.go' : 'dashboard/src/views/FixTracking.tsx',
      diff: null,
    },
  ];
}

function makeIssue(index: number, status: FixTrackingStatus): FixTrackingIssueRecord {
  const createdAt = randomDateWithinLastDays(30);
  const severity = pickSeverity(index);
  const category = pickCategory(index);
  const page = randomItem(PAGES);
  const action = randomItem(ACTIONS);
  const errorType = randomItem(ERROR_TYPES);

  const issue: FixTrackingIssueRecord = {
    id: index + 1,
    title: `${randomItem(TITLE_PREFIX)} issue on ${page}`,
    page,
    action,
    error_type: errorType,
    error_text: `${errorType.replace('_', ' ')} detected while running ${action} on ${page}.`,
    category,
    severity,
    status,
    is_fixed: status === 'fixed' || status === 'verified',
    notes: status === 'open' ? '' : 'Triaged and assigned to owner.',
    created_at: createdAt.toISOString(),
    suggestions: buildSuggestions(index + 1, category),
  };

  if (status === 'fixed' || status === 'verified') {
    const minHours = status === 'verified' ? 2 : 1;
    const maxHours = status === 'verified' ? 96 : 72;
    const resolutionHours = randomInt(minHours, maxHours);
    const resolvedAt = new Date(createdAt.getTime() + resolutionHours * 60 * 60 * 1000);

    issue.resolution_hours = resolutionHours;
    issue.resolved_at = resolvedAt.toISOString();
    issue.fix_commit_url = `https://github.com/example-org/qa-automation-tools/commit/${Math.random().toString(16).slice(2, 14)}`;
    issue.fix_pr_url = `https://github.com/example-org/qa-automation-tools/pull/${randomInt(100, 999)}`;

    if (status === 'verified') {
      issue.notes = `Verified by QA after ${resolutionHours}h. Regression checks passed.`;
    } else {
      issue.notes = `Fix deployed in ${resolutionHours}h and pending QA verification.`;
    }
  }

  if (status === 'in_progress') {
    issue.notes = 'Investigation in progress. Root cause identified.';
  }

  return issue;
}

export function generateFixTrackingMockData(count = 100): FixTrackingIssueRecord[] {
  const safeCount = Math.max(1, Math.floor(count));
  const statuses = buildStatusPool(safeCount);
  const issues = statuses.map((status, idx) => makeIssue(idx, status));

  return issues.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
}

export function calculateFixTrackingAnalytics(items: FixTrackingIssueRecord[]): FixTrackingAnalytics {
  const issuesBySeverity: Record<FixTrackingSeverity, number> = {
    low: 0,
    medium: 0,
    high: 0,
    critical: 0,
  };

  let fixedCount = 0;
  let verifiedCount = 0;
  let openCount = 0;
  let inProgressCount = 0;
  let totalResolutionHours = 0;
  let resolvedTotal = 0;

  for (const issue of items) {
    issuesBySeverity[issue.severity] += 1;

    switch (issue.status) {
      case 'fixed':
        fixedCount += 1;
        break;
      case 'verified':
        verifiedCount += 1;
        break;
      case 'open':
        openCount += 1;
        break;
      case 'in_progress':
        inProgressCount += 1;
        break;
    }

    if (typeof issue.resolution_hours === 'number') {
      totalResolutionHours += issue.resolution_hours;
      resolvedTotal += 1;
    }
  }

  const resolvedCount = fixedCount + verifiedCount;

  return {
    totalIssues: items.length,
    fixedCount,
    verifiedCount,
    openCount,
    inProgressCount,
    fixRatePercentage: items.length > 0 ? Math.round((resolvedCount / items.length) * 100) : 0,
    averageTimeToResolveHours: resolvedTotal > 0 ? Number((totalResolutionHours / resolvedTotal).toFixed(1)) : 0,
    issuesBySeverity,
  };
}

function compareBySeverity(a: FixTrackingSeverity, b: FixTrackingSeverity): number {
  const rank: Record<FixTrackingSeverity, number> = {
    critical: 4,
    high: 3,
    medium: 2,
    low: 1,
  };
  return rank[a] - rank[b];
}

function applyQuery(items: FixTrackingIssueRecord[], query: IssueQuery): FixTrackingIssueRecord[] {
  let filtered = [...items];

  if (typeof query.isFixed === 'boolean') {
    filtered = filtered.filter((issue) => issue.is_fixed === query.isFixed);
  }

  if (query.category) {
    filtered = filtered.filter((issue) => issue.category === query.category);
  }

  if (query.search) {
    const keyword = query.search.toLowerCase();
    filtered = filtered.filter((issue) => {
      return [issue.title, issue.page, issue.action, issue.error_text, issue.error_type]
        .join(' ')
        .toLowerCase()
        .includes(keyword);
    });
  }

  switch (query.sortBy) {
    case 'date_oldest':
      filtered.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
      break;
    case 'severity_high':
      filtered.sort((a, b) => compareBySeverity(b.severity, a.severity));
      break;
    case 'severity_low':
      filtered.sort((a, b) => compareBySeverity(a.severity, b.severity));
      break;
    case 'date_newest':
    default:
      filtered.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
      break;
  }

  return filtered;
}

function parseIssuesUrl(url: string): IssueQuery {
  const parsed = new URL(url, window.location.origin);
  const search = parsed.searchParams;

  const page = Math.max(1, Number(search.get('page') || '1'));
  const limit = Math.max(1, Number(search.get('limit') || '20'));

  const sortByRaw = search.get('sort_by') || 'date_newest';
  const sortBy: IssueQuery['sortBy'] =
    sortByRaw === 'date_oldest' || sortByRaw === 'severity_high' || sortByRaw === 'severity_low'
      ? sortByRaw
      : 'date_newest';

  const isFixedRaw = search.get('is_fixed');
  const isFixed = isFixedRaw === null ? undefined : isFixedRaw === 'true';

  const categoryRaw = search.get('category');
  const category: FixTrackingCategory | undefined =
    categoryRaw === 'frontend' || categoryRaw === 'backend' || categoryRaw === 'unknown'
      ? categoryRaw
      : undefined;

  const keyword = (search.get('search') || '').trim();

  return {
    page,
    limit,
    sortBy,
    isFixed,
    category,
    search: keyword || undefined,
  };
}

function normalizeStatus(issue: FixTrackingIssueRecord): FixTrackingStatus {
  if (issue.status === 'verified') {
    return 'verified';
  }
  return issue.is_fixed ? 'fixed' : 'open';
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      'Content-Type': 'application/json',
    },
  });
}

function parseRequestBody(input: RequestInfo | URL, init?: RequestInit): Promise<unknown> {
  if (init?.body && typeof init.body === 'string') {
    try {
      return Promise.resolve(JSON.parse(init.body));
    } catch {
      return Promise.resolve({});
    }
  }

  if (input instanceof Request) {
    return input
      .clone()
      .text()
      .then((text) => {
        if (!text) return {};
        try {
          return JSON.parse(text);
        } catch {
          return {};
        }
      });
  }

  return Promise.resolve({});
}

function toUrlString(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input;
  if (input instanceof URL) return input.toString();
  return input.url;
}

async function waitForLatency(minLatencyMs: number, maxLatencyMs: number): Promise<void> {
  const delay = randomInt(minLatencyMs, Math.max(minLatencyMs, maxLatencyMs));
  return new Promise((resolve) => setTimeout(resolve, delay));
}

export function installFixTrackingMockApi(options: FixTrackingMockApiOptions = {}): () => void {
  const count = options.count ?? 100;
  const minLatencyMs = options.minLatencyMs ?? 300;
  const maxLatencyMs = options.maxLatencyMs ?? 900;

  let store = generateFixTrackingMockData(count);
  const originalFetch = window.fetch.bind(window);

  window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = toUrlString(input);
    const method = (init?.method || (input instanceof Request ? input.method : 'GET')).toUpperCase();

    if (!url.includes('/api/issues')) {
      return originalFetch(input, init);
    }

    await waitForLatency(minLatencyMs, maxLatencyMs);

    if (url.includes('/api/issues/status') && method === 'POST') {
      const body = (await parseRequestBody(input, init)) as { id?: number; is_fixed?: boolean };
      const id = Number(body.id);
      if (!Number.isFinite(id)) {
        return jsonResponse({ error: 'invalid id' }, 400);
      }

      const target = store.find((item) => item.id === id);
      if (!target) {
        return jsonResponse({ error: 'issue not found' }, 404);
      }

      const nextIsFixed = Boolean(body.is_fixed);
      target.is_fixed = nextIsFixed;
      target.status = nextIsFixed ? normalizeStatus({ ...target, is_fixed: true }) : 'open';

      if (!nextIsFixed) {
        target.resolved_at = undefined;
        target.fix_commit_url = undefined;
        target.fix_pr_url = undefined;
        target.resolution_hours = undefined;
      } else if (!target.resolved_at) {
        target.status = randomInt(0, 100) > 75 ? 'verified' : 'fixed';
        const createdAt = new Date(target.created_at).getTime();
        const resolutionHours = randomInt(1, 96);
        target.resolution_hours = resolutionHours;
        target.resolved_at = new Date(Math.max(Date.now(), createdAt + resolutionHours * 60 * 60 * 1000)).toISOString();
        target.fix_commit_url = `https://github.com/example-org/qa-automation-tools/commit/${Math.random().toString(16).slice(2, 14)}`;
        target.fix_pr_url = `https://github.com/example-org/qa-automation-tools/pull/${randomInt(500, 1999)}`;
      }

      return jsonResponse({ id: target.id, is_fixed: target.is_fixed, status: target.status });
    }

    if (url.includes('/api/issues/notes') && method === 'POST') {
      const body = (await parseRequestBody(input, init)) as { id?: number; notes?: string };
      const id = Number(body.id);
      if (!Number.isFinite(id)) {
        return jsonResponse({ error: 'invalid id' }, 400);
      }

      const target = store.find((item) => item.id === id);
      if (!target) {
        return jsonResponse({ error: 'issue not found' }, 404);
      }

      target.notes = (body.notes || '').trim();
      return jsonResponse({ id: target.id, notes: target.notes });
    }

    if (url.includes('/api/issues') && method === 'GET') {
      const query = parseIssuesUrl(url);
      const filtered = applyQuery(store, query);
      const total = filtered.length;
      const totalPages = Math.max(1, Math.ceil(total / query.limit));
      const safePage = Math.min(query.page, totalPages);
      const start = (safePage - 1) * query.limit;
      const paged = filtered.slice(start, start + query.limit);

      const response: FixTrackingListResponse = {
        data: paged,
        pagination: {
          page: safePage,
          limit: query.limit,
          total,
          total_pages: totalPages,
        },
        analytics: calculateFixTrackingAnalytics(store),
      };

      return jsonResponse(response);
    }

    return originalFetch(input, init);
  };

  return () => {
    window.fetch = originalFetch;
    store = [];
  };
}
