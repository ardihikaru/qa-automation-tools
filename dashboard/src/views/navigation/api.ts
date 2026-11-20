// Types and fetchers of the navigation map (docs/designs/NAVIGATION_MAP.md section 6,
// ai-backend/internal/api/navigation.go). The server stays the authority.
import { api } from '../products/api';

export type Surface = 'web' | 'api';
export type RouteStatus = 'failing' | 'passing' | 'automated' | 'planned' | 'untested';

export const STATUSES: RouteStatus[] = ['failing', 'passing', 'automated', 'planned', 'untested'];

export const STATUS_LABEL: Record<RouteStatus, string> = {
  failing: 'Failing',
  passing: 'Passing',
  automated: 'Automated, no result',
  planned: 'Planned only',
  untested: 'No test case',
};

/** Dot / bar colour per status (kept with the labels so the legend and the tree agree). */
export const STATUS_DOT: Record<RouteStatus, string> = {
  failing: 'bg-red-500',
  passing: 'bg-green-500',
  automated: 'bg-sky-500',
  planned: 'bg-amber-400',
  untested: 'bg-gray-300',
};

export const STATUS_CHIP: Record<RouteStatus, string> = {
  failing: 'bg-red-100 text-red-800',
  passing: 'bg-green-100 text-green-800',
  automated: 'bg-sky-100 text-sky-800',
  planned: 'bg-amber-100 text-amber-800',
  untested: 'bg-gray-100 text-gray-700',
};

export interface NavRoute {
  key: string;
  surface: Surface;
  app: string;
  path: string;
  methods: string[];
  sources: string[];
  page_objects: string[];
  status: RouteStatus;
  cases: number;
  automated: number;
  planned: number;
  pass: number;
  fail: number;
  features: number;
  specs: number;
  observed_hits: number;
  last_observed_at: string | null;
  last_result_at: string | null;
}

export interface NavBuild {
  status: 'ok' | 'failed';
  sync_id: number | null;
  commit: string;
  built_at: string;
  error: string;
  routes: number;
  links: number;
  problems: string[];
}

export interface NextItem {
  case_code: string;
  priority: string;
  epic: string;
  route_key: string;
  path: string;
  app: string;
  surface: Surface;
  method: string;
  tier: 1 | 2;
  reason: string;
  example_case: string;
  example_spec: string;
}

export type Totals = Record<RouteStatus | 'routes' | 'observed' | 'unmapped_cases', number>;

export interface NavMap {
  product_id: number;
  build: NavBuild | null;
  apps: { surface: Surface; app: string; routes: number }[];
  totals: Totals;
  routes: NavRoute[];
  next: NextItem[];
}

export interface NavLastJob {
  id: string;
  type: string;
  status: string;
  completed_at: string | null;
  duration_ms: number | null;
}

export interface NavCase {
  code: string;
  scenario: string;
  status: string;
  automated: boolean;
  priority: string;
  epic: string;
  link_sources: string[];
  result: '' | 'pass' | 'fail' | 'skipped';
  run_at: string | null;
  run_ref: string;
  last_job: NavLastJob | null;
}

export interface NavRouteDetail extends Omit<NavRoute, 'cases' | 'features' | 'specs'> {
  /** The lists replace the counts of the map's route object. */
  features: { code: string; name: string; release_status: string }[];
  cases: NavCase[];
  related_cases: { code: string; status: string; automated: boolean }[];
  related_total: number;
  specs: string[];
  observations: { job_id: string; case_code: string; method: string; hits: number; observed_at: string }[];
}

export interface NavFilter {
  surface: '' | Surface;
  app: string;
  status: '' | RouteStatus;
  q: string;
}

export function getNavigation(pid: number, f: NavFilter, signal?: AbortSignal): Promise<NavMap> {
  const q = new URLSearchParams();
  if (f.surface) q.set('surface', f.surface);
  if (f.app) q.set('app', f.app);
  if (f.status) q.set('status', f.status);
  if (f.q) q.set('q', f.q);
  const qs = q.toString();
  return api<NavMap>(`/api/products/${pid}/navigation${qs ? `?${qs}` : ''}`, { signal });
}

export function getNavRoute(pid: number, key: string, signal?: AbortSignal): Promise<NavRouteDetail> {
  return api<NavRouteDetail>(`/api/products/${pid}/navigation/routes/${encodeURIComponent(key)}`, { signal });
}

export interface ProductLite {
  id: number;
  name: string;
  knowledge_format: string;
}

export function listProductsLite(signal?: AbortSignal): Promise<ProductLite[]> {
  return api<{ products?: ProductLite[] }>('/api/products', { signal }).then((r) => r.products ?? []);
}

/** The product shown without ?product=: the first one (by id) with a catalog format. */
export function defaultProduct(products: ProductLite[]): ProductLite | null {
  const sorted = [...products].sort((a, b) => a.id - b.id);
  return sorted.find((p) => p.knowledge_format) ?? sorted[0] ?? null;
}
