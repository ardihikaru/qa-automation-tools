import { useState, useEffect } from 'react';
import { 
  Zap, 
  Play, 
  CheckCircle, 
  XCircle, 
  Clock, 
  Search,
  Filter,
  BarChart3,
  AlertTriangle,
  ToggleLeft,
  ToggleRight,
  History,
  TrendingUp,
  Settings
} from 'lucide-react';

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

export default function SkillsView() {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillStats, setSkillStats] = useState<SkillStats[]>([]);
  const [skillLogs, setSkillLogs] = useState<SkillLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'skills' | 'stats' | 'logs'>('skills');
  const [search, setSearch] = useState('');
  const [filterSeverity, setFilterSeverity] = useState('all');
  const [filterPassed, setFilterPassed] = useState<string>('all');

  useEffect(() => {
    fetchData();
  }, [activeTab]);

  const fetchData = async () => {
    try {
      setLoading(true);
      
      if (activeTab === 'skills') {
        const res = await fetch('/api/skills');
        const data = await res.json();
        setSkills(Array.isArray(data) ? data : []);
      } else if (activeTab === 'stats') {
        const res = await fetch('/api/skills/stats');
        const data = await res.json();
        setSkillStats(Array.isArray(data) ? data : []);
      } else {
        const res = await fetch('/api/skills/logs?limit=50');
        const data = await res.json();
        setSkillLogs(Array.isArray(data) ? data : []);
      }
    } catch (error) {
      console.error('Failed to fetch data:', error);
    } finally {
      setLoading(false);
    }
  };

  const toggleSkillActive = async (skill: Skill) => {
    // Note: This would require a PATCH endpoint - for now just UI feedback
    console.log('Toggle skill:', skill.id, !skill.is_active);
  };

  const runSkill = async (skill: Skill) => {
    alert(`Running skill: ${skill.name}\n\nNote: This requires the QA automation system to be running.`);
  };

  const filteredSkills = skills.filter(skill => {
    const matchesSearch = !search || 
      skill.name?.toLowerCase().includes(search.toLowerCase()) ||
      skill.description?.toLowerCase().includes(search.toLowerCase());
    const matchesSeverity = filterSeverity === 'all' || skill.severity === filterSeverity;
    return matchesSearch && matchesSeverity;
  });

  const filteredStats = skillStats.filter(stat => {
    const matchesSearch = !search || 
      stat.skill_name?.toLowerCase().includes(search.toLowerCase());
    return matchesSearch;
  });

  const filteredLogs = skillLogs.filter(log => {
    const matchesSearch = !search || 
      log.page?.toLowerCase().includes(search.toLowerCase()) ||
      log.action?.toLowerCase().includes(search.toLowerCase());
    const matchesPassed = filterPassed === 'all' || 
      (filterPassed === 'passed' && log.is_passed) ||
      (filterPassed === 'failed' && !log.is_passed);
    return matchesSearch && matchesPassed;
  });

  const getSeverityBadge = (severity: string) => {
    switch (severity) {
      case 'high': return 'badge-error';
      case 'medium': return 'badge-warning';
      default: return 'badge-success';
    }
  };

  const getPassRateColor = (rate: number) => {
    if (rate >= 80) return 'text-green-600';
    if (rate >= 60) return 'text-yellow-600';
    return 'text-red-600';
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    try {
      return new Date(dateStr).toLocaleString();
    } catch {
      return dateStr;
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900">Skill Execution UI</h2>
        <p className="text-gray-600">Manage QA skills and view execution results</p>
      </div>

      {/* Tabs */}
      <div className="flex gap-2 mb-6">
        <button
          onClick={() => setActiveTab('skills')}
          className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
            activeTab === 'skills' 
              ? 'bg-primary-600 text-white' 
              : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
          }`}
        >
          <Zap className="w-4 h-4" />
          Skills
        </button>
        <button
          onClick={() => setActiveTab('stats')}
          className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
            activeTab === 'stats' 
              ? 'bg-primary-600 text-white' 
              : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
          }`}
        >
          <BarChart3 className="w-4 h-4" />
          Statistics
        </button>
        <button
          onClick={() => setActiveTab('logs')}
          className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
            activeTab === 'logs' 
              ? 'bg-primary-600 text-white' 
              : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
          }`}
        >
          <History className="w-4 h-4" />
          Execution Logs
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-col md:flex-row gap-4 mb-6">
        <div className="flex-1 relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            placeholder="Search skills..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          />
        </div>
        
        {activeTab === 'skills' && (
          <select
            value={filterSeverity}
            onChange={(e) => setFilterSeverity(e.target.value)}
            className="px-4 py-2 border border-gray-300 rounded-lg"
          >
            <option value="all">All Severities</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </select>
        )}

        {activeTab === 'logs' && (
          <select
            value={filterPassed}
            onChange={(e) => setFilterPassed(e.target.value)}
            className="px-4 py-2 border border-gray-300 rounded-lg"
          >
            <option value="all">All Results</option>
            <option value="passed">Passed</option>
            <option value="failed">Failed</option>
          </select>
        )}
      </div>

      {/* Stats Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Skills</div>
          <div className="text-2xl font-bold">{skills.length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Active Skills</div>
          <div className="text-2xl font-bold text-green-600">
            {skills.filter(s => s.is_active).length}
          </div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Runs</div>
          <div className="text-2xl font-bold">
            {skillStats.reduce((a, s) => a + s.total_runs, 0)}
          </div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Overall Pass Rate</div>
          <div className="text-2xl font-bold">
            {skillStats.length > 0 
              ? Math.round(skillStats.reduce((a, s) => a + s.pass_rate * s.total_runs, 0) / 
                         skillStats.reduce((a, s) => a + s.total_runs, 0) * 100)
              : 0}%
          </div>
        </div>
      </div>

      {/* Skills List Tab */}
      {activeTab === 'skills' && (
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">QA Skills ({filteredSkills.length})</h3>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Skill</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Severity</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {filteredSkills.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-6 py-8 text-center text-gray-500">
                      No skills found
                    </td>
                  </tr>
                ) : (
                  filteredSkills.map((skill) => (
                    <tr key={skill.id} className="hover:bg-gray-50">
                      <td className="px-6 py-4">
                        <button
                          onClick={() => toggleSkillActive(skill)}
                          className="text-gray-500 hover:text-primary-600"
                        >
                          {skill.is_active ? (
                            <ToggleRight className="w-8 h-8 text-green-600" />
                          ) : (
                            <ToggleLeft className="w-8 h-8 text-gray-400" />
                          )}
                        </button>
                      </td>
                      <td className="px-6 py-4">
                        <div className="font-medium text-gray-900">{skill.name}</div>
                        <div className="text-sm text-gray-500">{skill.description}</div>
                        {skill.is_builtin && (
                          <span className="badge badge-primary mt-1">Built-in</span>
                        )}
                      </td>
                      <td className="px-6 py-4 text-gray-600">{skill.check_type}</td>
                      <td className="px-6 py-4">
                        <span className={`badge ${getSeverityBadge(skill.severity)}`}>
                          {skill.severity}
                        </span>
                      </td>
                      <td className="px-6 py-4">
                        <button
                          onClick={() => runSkill(skill)}
                          className="btn btn-primary flex items-center gap-2"
                        >
                          <Play className="w-4 h-4" />
                          Run
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Statistics Tab */}
      {activeTab === 'stats' && (
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Skill Performance ({filteredStats.length})</h3>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Skill</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Total Runs</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Passed</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Failed</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Pass Rate</th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Last Run</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {filteredStats.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="px-6 py-8 text-center text-gray-500">
                      No statistics available
                    </td>
                  </tr>
                ) : (
                  filteredStats.map((stat) => (
                    <tr key={stat.skill_id} className="hover:bg-gray-50">
                      <td className="px-6 py-4 font-medium text-gray-900">{stat.skill_name}</td>
                      <td className="px-6 py-4 text-gray-600">{stat.total_runs}</td>
                      <td className="px-6 py-4 text-green-600">{stat.passed_runs}</td>
                      <td className="px-6 py-4 text-red-600">{stat.failed_runs}</td>
                      <td className="px-6 py-4">
                        <span className={`font-bold ${getPassRateColor(stat.pass_rate * 100)}`}>
                          {Math.round(stat.pass_rate * 100)}%
                        </span>
                        <div className="w-24 bg-gray-200 rounded-full h-2 mt-1">
                          <div 
                            className={`h-2 rounded-full ${
                              stat.pass_rate >= 0.8 ? 'bg-green-600' :
                              stat.pass_rate >= 0.6 ? 'bg-yellow-600' : 'bg-red-600'
                            }`}
                            style={{ width: `${stat.pass_rate * 100}%` }}
                          ></div>
                        </div>
                      </td>
                      <td className="px-6 py-4 text-gray-500">
                        {stat.last_run ? formatDate(stat.last_run) : 'N/A'}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Execution Logs Tab */}
      {activeTab === 'logs' && (
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Recent Execution Logs ({filteredLogs.length})</h3>
          </div>
          <div className="divide-y divide-gray-200 max-h-96 overflow-y-auto">
            {filteredLogs.length === 0 ? (
              <div className="p-8 text-center text-gray-500">No execution logs available</div>
            ) : (
              filteredLogs.map((log) => (
                <div key={log.id} className="p-4 hover:bg-gray-50">
                  <div className="flex items-start justify-between">
                    <div className="flex items-start gap-3">
                      <div className={`p-2 rounded-lg ${log.is_passed ? 'bg-green-100' : 'bg-red-100'}`}>
                        {log.is_passed ? (
                          <CheckCircle className="w-4 h-4 text-green-600" />
                        ) : (
                          <XCircle className="w-4 h-4 text-red-600" />
                        )}
                      </div>
                      <div>
                        <div className="font-medium text-gray-900">
                          {log.skill_name || `Skill #${log.skill_id}`}
                        </div>
                        <div className="text-sm text-gray-500">
                          {log.action} on {log.page}
                        </div>
                        {log.error_message && (
                          <div className="mt-2 p-2 bg-red-50 rounded text-sm text-red-700">
                            {log.error_message}
                          </div>
                        )}
                      </div>
                    </div>
                    <div className="text-right text-sm text-gray-500">
                      <Clock className="w-4 h-4 inline mr-1" />
                      {formatDate(log.created_at)}
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}