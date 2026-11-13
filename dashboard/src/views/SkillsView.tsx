import { useEffect, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { usePageHeader } from '../components/pageHeader';
import { Zap, BarChart3, History, Search, Play, CheckCircle, XCircle, Clock } from 'lucide-react';
import AssistantList from './assistants/AssistantList';
import { parseProductParam, useProducts } from './assistants/api';
import { ProductSelect, Segmented } from './assistants/ui';

interface Skill {
  id: number;
  name: string;
  description: string;
  check_type: string;
  check_query: string;
  severity: string;
  is_builtin: boolean;
  is_active: boolean;
  created_at: string;
  /** null = global (applies to every product). */
  product_id?: number | null;
}

interface SkillStats {
  skill_id: number;
  skill_name: string;
  total_runs: number;
  passed_runs: number;
  failed_runs: number;
  pass_rate: number;
  last_run?: string;
}

interface SkillLog {
  id: number;
  skill_id: number;
  skill_name?: string;
  page: string;
  action: string;
  result: string;
  is_passed: boolean;
  error_message?: string;
  created_at: string;
}

interface Pagination {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

interface SkillsResponse {
  data?: Skill[];
  pagination?: Pagination;
}
interface SkillStatsResponse {
  data?: SkillStats[];
  pagination?: Pagination;
}
interface SkillLogsResponse {
  data?: SkillLog[];
  pagination?: Pagination;
}

const DEFAULT_LIMIT = 10;

function SkillsSkeleton() {
  return (
    <div className="card p-4 animate-pulse">
      <div className="h-5 w-40 bg-gray-200 rounded mb-4" />
      {Array.from({ length: 5 }).map((_, idx) => (
        <div key={idx} className="h-12 bg-gray-100 rounded mb-2" />
      ))}
    </div>
  );
}

type SkillsMode = 'checks' | 'repo';

export default function SkillsView() {
  const [searchParams, setSearchParams] = useSearchParams();
  const { products, error: productsError } = useProducts();

  // `?view=repo` shows the skills synced from automation repos; anything else shows the QA checks.
  const mode: SkillsMode = searchParams.get('view') === 'repo' ? 'repo' : 'checks';
  // `?product=`: '' = all, 'global' = global QA checks only, or a product id.
  const productRaw = searchParams.get('product') ?? '';
  const productId = parseProductParam(productRaw);
  const productParam = productRaw === 'global' ? 'global' : productId ? String(productId) : '';
  const productName = (id: number | null | undefined) => products.find((p) => p.id === id)?.name ?? `Product #${id}`;

  const activeTab = (searchParams.get('tab') as 'skills' | 'stats' | 'logs') || 'skills';
  const searchParam = searchParams.get('search') || '';
  const severityParam = searchParams.get('severity') || 'all';
  const passedParam = searchParams.get('passed') || 'all';
  const pageParam = Number(searchParams.get('page') || '1');

  // Sub-views get their own title under a "Skills" crumb; the QA checks list keeps the nav title.
  const subTitle = mode === 'repo' ? 'Repo skills' : activeTab === 'stats' ? 'Statistics' : activeTab === 'logs' ? 'Execution logs' : null;
  usePageHeader(subTitle ? { title: subTitle, crumbs: [{ label: 'Skills', to: '/skills' }] } : null);

  const [searchInput, setSearchInput] = useState(searchParam);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillStats, setSkillStats] = useState<SkillStats[]>([]);
  const [skillLogs, setSkillLogs] = useState<SkillLog[]>([]);
  const [skillsPagination, setSkillsPagination] = useState<Pagination>({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });
  const [statsPagination, setStatsPagination] = useState<Pagination>({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });
  const [logsPagination, setLogsPagination] = useState<Pagination>({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });

  const [loadingSkills, setLoadingSkills] = useState(false);
  const [loadingStats, setLoadingStats] = useState(false);
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [skillsError, setSkillsError] = useState<string | null>(null);
  // Only the latest skills request may write state, so a slow reply never overwrites a newer filter.
  const skillsSeq = useRef(0);

  useEffect(() => {
    setSearchInput(searchParam);
  }, [searchParam]);

  useEffect(() => {
    if (mode === 'repo') return;
    if (activeTab === 'skills') {
      fetchSkills();
    } else if (activeTab === 'stats') {
      fetchStats();
    } else {
      fetchLogs();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, activeTab, searchParam, severityParam, passedParam, pageParam, productParam]);

  const updateParams = (patch: Record<string, string>) => {
    const next = new URLSearchParams(searchParams.toString());
    Object.entries(patch).forEach(([k, v]) => {
      if (!v) next.delete(k);
      else next.set(k, v);
    });
    setSearchParams(next);
  };

  const fetchSkills = async () => {
    const seq = ++skillsSeq.current;
    try {
      setLoadingSkills(true);
      setSkillsError(null);
      const params = new URLSearchParams();
      params.set('page', String(Math.max(1, pageParam)));
      params.set('limit', String(DEFAULT_LIMIT));
      if (searchParam.trim()) params.set('search', searchParam.trim());
      if (severityParam !== 'all') params.set('severity', severityParam);
      if (productParam) params.set('product_id', productParam);
      params.set('sort_by', 'name_desc');

      const res = await fetch(`/api/skills?${params.toString()}`, { credentials: 'include' });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body?.error || `Could not load QA checks (${res.status})`);
      }
      const payload: SkillsResponse | Skill[] = await res.json();
      if (seq !== skillsSeq.current) return;

      if (Array.isArray(payload)) {
        setSkills(payload);
        setSkillsPagination({ page: pageParam, limit: DEFAULT_LIMIT, total: payload.length, total_pages: 1 });
      } else {
        const data = Array.isArray(payload.data) ? payload.data : [];
        const pg = payload.pagination;
        setSkills(data);
        setSkillsPagination({
          page: pg?.page ?? pageParam,
          limit: pg?.limit ?? DEFAULT_LIMIT,
          total: pg?.total ?? data.length,
          total_pages: pg?.total_pages ?? 1,
        });
      }
    } catch (error) {
      if (seq !== skillsSeq.current) return;
      console.error('Failed to fetch skills:', error);
      setSkills([]);
      setSkillsPagination({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });
      setSkillsError(error instanceof Error ? error.message : 'Could not load QA checks');
    } finally {
      if (seq === skillsSeq.current) setLoadingSkills(false);
    }
  };

  const fetchStats = async () => {
    try {
      setLoadingStats(true);
      const params = new URLSearchParams();
      params.set('page', String(Math.max(1, pageParam)));
      params.set('limit', String(DEFAULT_LIMIT));
      if (searchParam.trim()) params.set('search', searchParam.trim());
      params.set('sort_by', 'total_runs_desc');

      const res = await fetch(`/api/skills/stats?${params.toString()}`, { credentials: 'include' });
      const payload: SkillStatsResponse | SkillStats[] = await res.json();
      if (Array.isArray(payload)) {
        setSkillStats(payload);
        setStatsPagination({ page: pageParam, limit: DEFAULT_LIMIT, total: payload.length, total_pages: 1 });
      } else {
        const data = Array.isArray(payload.data) ? payload.data : [];
        const pg = payload.pagination;
        setSkillStats(data);
        setStatsPagination({
          page: pg?.page ?? pageParam,
          limit: pg?.limit ?? DEFAULT_LIMIT,
          total: pg?.total ?? data.length,
          total_pages: pg?.total_pages ?? 1,
        });
      }
    } catch (error) {
      console.error('Failed to fetch skill stats:', error);
      setSkillStats([]);
      setStatsPagination({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });
    } finally {
      setLoadingStats(false);
    }
  };

  const fetchLogs = async () => {
    try {
      setLoadingLogs(true);
      const params = new URLSearchParams();
      params.set('page', String(Math.max(1, pageParam)));
      params.set('limit', String(DEFAULT_LIMIT));
      if (passedParam === 'passed') params.set('passed', 'true');
      if (passedParam === 'failed') params.set('passed', 'false');
      if (searchParam.trim()) params.set('search', searchParam.trim());
      const res = await fetch(`/api/skills/logs?${params.toString()}`, { credentials: 'include' });
      const payload: SkillLogsResponse | SkillLog[] = await res.json();
      if (Array.isArray(payload)) {
        setSkillLogs(payload);
        setLogsPagination({ page: pageParam, limit: DEFAULT_LIMIT, total: payload.length, total_pages: 1 });
      } else {
        const data = Array.isArray(payload.data) ? payload.data : [];
        const pg = payload.pagination;
        setSkillLogs(data);
        setLogsPagination({
          page: pg?.page ?? pageParam,
          limit: pg?.limit ?? DEFAULT_LIMIT,
          total: pg?.total ?? data.length,
          total_pages: pg?.total_pages ?? 1,
        });
      }
    } catch (error) {
      console.error('Failed to fetch skill logs:', error);
      setSkillLogs([]);
      setLogsPagination({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });
    } finally {
      setLoadingLogs(false);
    }
  };

  const applySearch = () => {
    updateParams({ search: searchInput.trim(), page: '1' });
  };

  const setTab = (tab: 'skills' | 'stats' | 'logs') => {
    updateParams({ tab, page: '1' });
  };

  const setMode = (m: SkillsMode) => {
    // Each view keeps its own search and paging; the product filter carries over.
    const next = new URLSearchParams();
    if (m === 'repo') next.set('view', 'repo');
    if (productId) next.set('product', String(productId));
    else if (m === 'checks' && productParam === 'global') next.set('product', 'global');
    setSearchParams(next);
  };

  const setProduct = (v: string) => updateParams({ product: v, page: '' });

  const productSelect = (
    <ProductSelect
      value={mode === 'repo' && productParam === 'global' ? '' : productParam}
      onChange={setProduct}
      products={products}
      failed={productsError}
      extra={mode === 'checks' ? [{ value: 'global', label: 'Global checks only' }] : undefined}
    />
  );

  return (
    <div className="min-w-0">
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <p className="text-gray-600">
            {mode === 'checks'
              ? 'QA checks the test runner applies to pages. Global checks apply to every product; a product can add its own.'
              : 'Claude skills defined in each product\u2019s automation repo (.claude/skills), refreshed by the product sync.'}
          </p>
        </div>
        <Segmented<SkillsMode>
          label="Skill source"
          value={mode}
          onChange={setMode}
          options={[
            { value: 'checks', label: 'QA checks' },
            { value: 'repo', label: 'Repo skills' },
          ]}
        />
      </div>

      {mode === 'repo' ? (
        <AssistantList
          kinds={['skill']}
          one="skill"
          plural="skills"
          productId={productId}
          productName={productId ? productName(productId) : undefined}
          toolbar={productSelect}
        />
      ) : (
      <>
      <div className="flex gap-2 mb-6">
        <button onClick={() => setTab('skills')} className={`px-4 py-2 rounded-lg flex items-center gap-2 ${activeTab === 'skills' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'}`}><Zap className="w-4 h-4" />Skills</button>
        <button onClick={() => setTab('stats')} className={`px-4 py-2 rounded-lg flex items-center gap-2 ${activeTab === 'stats' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'}`}><BarChart3 className="w-4 h-4" />Statistics</button>
        <button onClick={() => setTab('logs')} className={`px-4 py-2 rounded-lg flex items-center gap-2 ${activeTab === 'logs' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'}`}><History className="w-4 h-4" />Execution Logs</button>
      </div>

      <div className="card mb-6 p-4">
        <div className="grid grid-cols-1 md:grid-cols-5 gap-3">
          {activeTab === 'skills' && <div className="md:col-span-5">{productSelect}</div>}
          <div className="md:col-span-2 relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') applySearch();
              }}
              className="w-full pl-10 pr-3 py-2 border rounded-lg"
              placeholder={activeTab === 'logs' ? 'Search logs by page keyword...' : 'Search skills...'}
            />
          </div>

          {activeTab === 'skills' && (
            <select value={severityParam} onChange={(e) => updateParams({ severity: e.target.value, page: '1' })} className="px-3 py-2 border rounded-lg">
              <option value="all">All Severities</option>
              <option value="high">High</option>
              <option value="medium">Medium</option>
              <option value="low">Low</option>
            </select>
          )}

          {activeTab === 'logs' && (
            <select value={passedParam} onChange={(e) => updateParams({ passed: e.target.value })} className="px-3 py-2 border rounded-lg">
              <option value="all">All Results</option>
              <option value="passed">Passed</option>
              <option value="failed">Failed</option>
            </select>
          )}

          <button className="btn btn-primary" onClick={applySearch}>Search</button>
        </div>
      </div>

      {activeTab === 'skills' && (
        <div className="card overflow-hidden">
          <div className="card-header">
            <h3 className="font-semibold">
              QA checks <span className="tabular-nums text-gray-500 font-normal">{skillsPagination.total}</span>
            </h3>
          </div>
          {skillsError && (
            <div role="alert" className="mx-4 mt-4 text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2 flex items-start justify-between gap-3 break-words">
              <span className="min-w-0">{skillsError}</span>
              <button type="button" className="text-red-700 underline shrink-0" onClick={() => void fetchSkills()}>
                Retry
              </button>
            </div>
          )}
          {loadingSkills ? (
            <div className="p-4"><SkillsSkeleton /></div>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50 border-b">
                    <tr>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Name</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Applies to</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Type</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Severity</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Status</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Action</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {skills.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-6 py-10 text-center text-gray-500">
                          {skillsError
                            ? 'QA checks could not be loaded.'
                            : searchParam || severityParam !== 'all'
                              ? 'No QA checks match these filters.'
                              : productParam === 'global'
                                ? 'There are no global QA checks yet.'
                                : productId
                                  ? `No QA checks apply to ${productName(productId)} yet.`
                                  : 'There are no QA checks yet.'}
                        </td>
                      </tr>
                    ) : (
                      skills.map((skill) => (
                        <tr key={skill.id}>
                          <td className="px-4 py-3">
                            <div className="font-medium text-gray-900 break-words">{skill.name || 'Unnamed skill'}</div>
                            <div className="text-xs text-gray-500 break-words">{skill.description || 'No description'}</div>
                          </td>
                          <td className="px-4 py-3 text-sm">
                            {skill.product_id ? (
                              <span className="badge bg-indigo-50 text-indigo-700 max-w-[14rem] truncate" title={productName(skill.product_id)}>
                                {productName(skill.product_id)}
                              </span>
                            ) : (
                              <span className="badge bg-gray-100 text-gray-700" title="Applies to every product">
                                Global
                              </span>
                            )}
                          </td>
                          <td className="px-4 py-3 text-sm text-gray-700">{skill.check_type || 'unknown'}</td>
                          <td className="px-4 py-3 text-sm text-gray-700">{skill.severity || 'unknown'}</td>
                          <td className="px-4 py-3 text-sm">{skill.is_active ? <span className="badge badge-success">active</span> : <span className="badge">inactive</span>}</td>
                          <td className="px-4 py-3"><button className="btn btn-secondary text-xs px-2 py-1 flex items-center gap-1"><Play className="w-3 h-3" /> Run</button></td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
              <div className="border-t bg-gray-50 px-4 py-3 flex items-center justify-between">
                <div className="text-sm text-gray-600">Page {skillsPagination.page} / {Math.max(1, skillsPagination.total_pages)} | Total {skillsPagination.total}</div>
                <div className="flex gap-2">
                  <button
                    className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50"
                    disabled={skillsPagination.page <= 1 || loadingSkills}
                    onClick={() => updateParams({ page: String(Math.max(1, skillsPagination.page - 1)) })}
                  >Prev</button>
                  <button
                    className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50"
                    disabled={skillsPagination.page >= skillsPagination.total_pages || loadingSkills}
                    onClick={() => updateParams({ page: String(Math.min(skillsPagination.total_pages || 1, skillsPagination.page + 1)) })}
                  >Next</button>
                </div>
              </div>
            </>
          )}
        </div>
      )}

      {activeTab === 'stats' && (
        <div className="card overflow-hidden">
          <div className="card-header">
            <h3 className="font-semibold">Statistics ({statsPagination.total})</h3>
          </div>
          {loadingStats ? (
            <div className="p-4"><SkillsSkeleton /></div>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50 border-b">
                    <tr>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Skill</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Runs</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Passed</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Failed</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Pass Rate</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {skillStats.length === 0 ? (
                      <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No stats found</td></tr>
                    ) : (
                      skillStats.map((s) => (
                        <tr key={s.skill_id}>
                          <td className="px-4 py-3 text-sm text-gray-900">{s.skill_name || 'Unknown skill'}</td>
                          <td className="px-4 py-3 text-sm">{s.total_runs}</td>
                          <td className="px-4 py-3 text-sm text-green-700">{s.passed_runs}</td>
                          <td className="px-4 py-3 text-sm text-red-700">{s.failed_runs}</td>
                          <td className="px-4 py-3 text-sm">{Math.round((s.pass_rate || 0) * 100)}%</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
              <div className="border-t bg-gray-50 px-4 py-3 flex items-center justify-between">
                <div className="text-sm text-gray-600">Page {statsPagination.page} / {Math.max(1, statsPagination.total_pages)} | Total {statsPagination.total}</div>
                <div className="flex gap-2">
                  <button
                    className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50"
                    disabled={statsPagination.page <= 1 || loadingStats}
                    onClick={() => updateParams({ page: String(Math.max(1, statsPagination.page - 1)) })}
                  >Prev</button>
                  <button
                    className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50"
                    disabled={statsPagination.page >= statsPagination.total_pages || loadingStats}
                    onClick={() => updateParams({ page: String(Math.min(statsPagination.total_pages || 1, statsPagination.page + 1)) })}
                  >Next</button>
                </div>
              </div>
            </>
          )}
        </div>
      )}

      {activeTab === 'logs' && (
        <div className="card overflow-hidden">
          <div className="card-header">
            <h3 className="font-semibold">Execution Logs ({logsPagination.total})</h3>
          </div>
          {loadingLogs ? (
            <div className="p-4"><SkillsSkeleton /></div>
          ) : (
            <>
              <div className="divide-y divide-gray-200">
                {skillLogs.length === 0 ? (
                  <div className="p-8 text-center text-gray-500">No logs found</div>
                ) : (
                  skillLogs.map((log) => (
                    <div key={log.id} className="p-4 flex items-start justify-between gap-3 hover:bg-gray-50">
                      <div>
                        <div className="font-medium text-gray-900">{log.skill_name || `Skill #${log.skill_id}`}</div>
                        <div className="text-sm text-gray-600">{log.action || 'Unknown action'} on {log.page || 'Unknown page'}</div>
                        {log.error_message && <div className="text-xs text-red-600 mt-1">{log.error_message}</div>}
                      </div>
                      <div className="text-right text-sm text-gray-600">
                        <div className="flex items-center gap-1 justify-end">
                          {log.is_passed ? <CheckCircle className="w-4 h-4 text-green-600" /> : <XCircle className="w-4 h-4 text-red-600" />}
                          <span>{log.is_passed ? 'passed' : 'failed'}</span>
                        </div>
                        <div className="flex items-center gap-1 justify-end text-xs mt-1">
                          <Clock className="w-3 h-3" />{log.created_at ? new Date(log.created_at).toLocaleString() : 'N/A'}
                        </div>
                      </div>
                    </div>
                  ))
                )}
              </div>
              <div className="border-t bg-gray-50 px-4 py-3 flex items-center justify-between">
                <div className="text-sm text-gray-600">Page {logsPagination.page} / {Math.max(1, logsPagination.total_pages)} | Total {logsPagination.total}</div>
                <div className="flex gap-2">
                  <button
                    className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50"
                    disabled={logsPagination.page <= 1 || loadingLogs}
                    onClick={() => updateParams({ page: String(Math.max(1, logsPagination.page - 1)) })}
                  >Prev</button>
                  <button
                    className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50"
                    disabled={logsPagination.page >= logsPagination.total_pages || loadingLogs}
                    onClick={() => updateParams({ page: String(Math.min(logsPagination.total_pages || 1, logsPagination.page + 1)) })}
                  >Next</button>
                </div>
              </div>
            </>
          )}
        </div>
      )}
      </>
      )}
    </div>
  );
}
