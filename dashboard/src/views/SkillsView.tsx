import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { Zap, BarChart3, History, Search, Play, CheckCircle, XCircle, Clock } from 'lucide-react';

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

export default function SkillsView() {
  const [searchParams, setSearchParams] = useSearchParams();

  const activeTab = (searchParams.get('tab') as 'skills' | 'stats' | 'logs') || 'skills';
  const searchParam = searchParams.get('search') || '';
  const severityParam = searchParams.get('severity') || 'all';
  const passedParam = searchParams.get('passed') || 'all';
  const pageParam = Number(searchParams.get('page') || '1');

  const [searchInput, setSearchInput] = useState(searchParam);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillStats, setSkillStats] = useState<SkillStats[]>([]);
  const [skillLogs, setSkillLogs] = useState<SkillLog[]>([]);
  const [skillsPagination, setSkillsPagination] = useState<Pagination>({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });

  const [loadingSkills, setLoadingSkills] = useState(false);
  const [loadingStats, setLoadingStats] = useState(false);
  const [loadingLogs, setLoadingLogs] = useState(false);

  useEffect(() => {
    setSearchInput(searchParam);
  }, [searchParam]);

  useEffect(() => {
    if (activeTab === 'skills') {
      fetchSkills();
    } else if (activeTab === 'stats') {
      fetchStats();
    } else {
      fetchLogs();
    }
  }, [activeTab, searchParam, severityParam, passedParam, pageParam]);

  const updateParams = (patch: Record<string, string>) => {
    const next = new URLSearchParams(searchParams.toString());
    Object.entries(patch).forEach(([k, v]) => {
      if (!v) next.delete(k);
      else next.set(k, v);
    });
    setSearchParams(next);
  };

  const fetchSkills = async () => {
    try {
      setLoadingSkills(true);
      const params = new URLSearchParams();
      params.set('page', String(Math.max(1, pageParam)));
      params.set('limit', String(DEFAULT_LIMIT));
      if (searchParam.trim()) params.set('search', searchParam.trim());
      if (severityParam !== 'all') params.set('severity', severityParam);
      params.set('sort_by', 'name_desc');

      const res = await fetch(`/api/skills?${params.toString()}`);
      const payload: SkillsResponse | Skill[] = await res.json();

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
      console.error('Failed to fetch skills:', error);
      setSkills([]);
      setSkillsPagination({ page: 1, limit: DEFAULT_LIMIT, total: 0, total_pages: 1 });
    } finally {
      setLoadingSkills(false);
    }
  };

  const fetchStats = async () => {
    try {
      setLoadingStats(true);
      const res = await fetch('/api/skills/stats');
      const data = await res.json();
      setSkillStats(Array.isArray(data) ? data : []);
    } catch (error) {
      console.error('Failed to fetch skill stats:', error);
      setSkillStats([]);
    } finally {
      setLoadingStats(false);
    }
  };

  const fetchLogs = async () => {
    try {
      setLoadingLogs(true);
      const params = new URLSearchParams();
      params.set('limit', '100');
      if (passedParam === 'passed') params.set('passed', 'true');
      if (passedParam === 'failed') params.set('passed', 'false');
      if (searchParam.trim()) params.set('page', searchParam.trim());
      const res = await fetch(`/api/skills/logs?${params.toString()}`);
      const data = await res.json();
      setSkillLogs(Array.isArray(data) ? data : []);
    } catch (error) {
      console.error('Failed to fetch skill logs:', error);
      setSkillLogs([]);
    } finally {
      setLoadingLogs(false);
    }
  };

  const filteredStats = useMemo(() => {
    const q = searchParam.trim().toLowerCase();
    if (!q) return skillStats;
    return skillStats.filter((s) => s.skill_name?.toLowerCase().includes(q));
  }, [skillStats, searchParam]);

  const applySearch = () => {
    updateParams({ search: searchInput.trim(), page: '1' });
  };

  const setTab = (tab: 'skills' | 'stats' | 'logs') => {
    updateParams({ tab, page: '1' });
  };

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900">Skill Execution UI</h2>
        <p className="text-gray-600">Server-driven skills table with URL-synced query state</p>
      </div>

      <div className="flex gap-2 mb-6">
        <button onClick={() => setTab('skills')} className={`px-4 py-2 rounded-lg flex items-center gap-2 ${activeTab === 'skills' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'}`}><Zap className="w-4 h-4" />Skills</button>
        <button onClick={() => setTab('stats')} className={`px-4 py-2 rounded-lg flex items-center gap-2 ${activeTab === 'stats' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'}`}><BarChart3 className="w-4 h-4" />Statistics</button>
        <button onClick={() => setTab('logs')} className={`px-4 py-2 rounded-lg flex items-center gap-2 ${activeTab === 'logs' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'}`}><History className="w-4 h-4" />Execution Logs</button>
      </div>

      <div className="card mb-6 p-4">
        <div className="grid grid-cols-1 md:grid-cols-5 gap-3">
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
            <h3 className="font-semibold">Skills ({skillsPagination.total})</h3>
          </div>
          {loadingSkills ? (
            <div className="p-4"><SkillsSkeleton /></div>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full">
                  <thead className="bg-gray-50 border-b">
                    <tr>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Name</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Type</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Severity</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Status</th>
                      <th className="px-4 py-3 text-left text-xs text-gray-500 uppercase">Action</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {skills.length === 0 ? (
                      <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No skills found</td></tr>
                    ) : (
                      skills.map((skill) => (
                        <tr key={skill.id}>
                          <td className="px-4 py-3">
                            <div className="font-medium text-gray-900">{skill.name || 'Unnamed skill'}</div>
                            <div className="text-xs text-gray-500">{skill.description || 'No description'}</div>
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
            <h3 className="font-semibold">Statistics ({filteredStats.length})</h3>
            <p className="text-xs text-amber-700 mt-1">Server-side search/pagination for stats is not available in current backend contract.</p>
          </div>
          {loadingStats ? (
            <div className="p-4"><SkillsSkeleton /></div>
          ) : (
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
                  {filteredStats.length === 0 ? (
                    <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No stats found</td></tr>
                  ) : (
                    filteredStats.map((s) => (
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
          )}
        </div>
      )}

      {activeTab === 'logs' && (
        <div className="card overflow-hidden">
          <div className="card-header">
            <h3 className="font-semibold">Execution Logs ({skillLogs.length})</h3>
            <p className="text-xs text-amber-700 mt-1">Server-side pagination metadata for logs is not available; using backend limit.</p>
          </div>
          {loadingLogs ? (
            <div className="p-4"><SkillsSkeleton /></div>
          ) : (
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
          )}
        </div>
      )}
    </div>
  );
}
