import { useState, useEffect } from 'react';
import { 
  Activity, 
  AlertTriangle, 
  CheckCircle, 
  XCircle, 
  Play, 
  FileText, 
  Bug,
  Zap,
  Clock,
  TrendingUp,
  BarChart3
} from 'lucide-react';

interface AnalyticsData {
  flows: number;
  issues: number;
  skills: number;
  goal_events: number;
  navigation_edges: number;
  action_sequences: number;
  total_records: number;
}

interface CostOverview {
  total_requests: number;
  total_tokens: number;
  total_cost: number;
  cache_hit_rate: number;
}

interface SkillStat {
  id: number;
  name: string;
  description: string;
  check_type: string;
  check_query: string;
  severity: string;
  is_builtin: boolean;
  is_active: boolean;
  created_at: string;
  pass_rate?: number;
  total_runs?: number;
  passed_runs?: number;
  failed_runs?: number;
}

interface RecentActivity {
  type: 'flow' | 'issue' | 'fix';
  title: string;
  description: string;
  timestamp: string;
}

interface IssueCount {
  total: number;
  open: number;
  fixed: number;
}

export default function QAInsights() {
  const [analytics, setAnalytics] = useState<AnalyticsData | null>(null);
  const [cost, setCost] = useState<CostOverview | null>(null);
  const [skillStats, setSkillStats] = useState<SkillStat[]>([]);
  const [issueCounts, setIssueCounts] = useState<IssueCount>({ total: 0, open: 0, fixed: 0 });
  const [loading, setLoading] = useState(true);
  const [recentActivity, setRecentActivity] = useState<RecentActivity[]>([]);

  useEffect(() => {
    fetchAllData();
  }, []);

  const fetchAllData = async () => {
    try {
      // Fetch analytics
      const analyticsRes = await fetch('/api/analytics');
      const analyticsData = await analyticsRes.json();
      setAnalytics(analyticsData);

      // Fetch cost overview
      const costRes = await fetch('/api/cost/overview');
      const costData = await costRes.json();
      setCost(costData);

      // Fetch skill stats
      const skillsRes = await fetch('/api/skills/stats');
      const skillsData = await skillsRes.json();
      setSkillStats(Array.isArray(skillsData) ? skillsData : []);

      // Fetch issue counts
      const allIssuesRes = await fetch('/api/issues?limit=1');
      const allIssuesData = await allIssuesRes.json();
      const pagination = allIssuesData?.pagination || {};
      
      const openRes = await fetch('/api/issues?is_fixed=false&limit=1');
      const openData = await openRes.json();
      const openPagination = openData?.pagination || {};

      const fixedRes = await fetch('/api/issues?is_fixed=true&limit=1');
      const fixedData = await fixedRes.json();
      const fixedPagination = fixedData?.pagination || {};

      setIssueCounts({
        total: pagination.total || 0,
        open: openPagination.total || 0,
        fixed: fixedPagination.total || 0
      });

      // Generate recent activity from available data
      setRecentActivity([
        { type: 'issue', title: 'Issue detected', description: `Open issues: ${openPagination.total || 0}`, timestamp: new Date().toISOString() },
        { type: 'fix', title: 'Fix applied', description: `Fixed issues: ${fixedPagination.total || 0}`, timestamp: new Date().toISOString() },
        { type: 'flow', title: 'Flows recorded', description: `Total flows: ${analyticsData?.flows || 0}`, timestamp: new Date().toISOString() },
      ]);

    } catch (error) {
      console.error('Failed to fetch data:', error);
    } finally {
      setLoading(false);
    }
  };

  const getPassRateColor = (rate: number) => {
    if (rate >= 80) return 'text-green-600';
    if (rate >= 60) return 'text-yellow-600';
    return 'text-red-600';
  };

  const getSeverityBadge = (severity: string) => {
    switch (severity) {
      case 'high': return 'badge-error';
      case 'medium': return 'badge-warning';
      default: return 'badge-success';
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
        <h2 className="text-2xl font-bold text-gray-900">QA Insights</h2>
        <p className="text-gray-600">Overview of QA automation activities and system health</p>
      </div>

      {/* Key Metrics Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">Total Flows</div>
              <div className="text-2xl font-bold">{analytics?.flows || 0}</div>
            </div>
            <div className="p-3 bg-blue-100 rounded-lg">
              <Activity className="w-6 h-6 text-blue-600" />
            </div>
          </div>
        </div>
        
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">Open Issues</div>
              <div className="text-2xl font-bold text-red-600">{issueCounts.open}</div>
            </div>
            <div className="p-3 bg-red-100 rounded-lg">
              <AlertTriangle className="w-6 h-6 text-red-600" />
            </div>
          </div>
        </div>
        
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">Active Skills</div>
              <div className="text-2xl font-bold">{analytics?.skills || 0}</div>
            </div>
            <div className="p-3 bg-purple-100 rounded-lg">
              <Zap className="w-6 h-6 text-purple-600" />
            </div>
          </div>
        </div>
        
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">Total Cost</div>
              <div className="text-2xl font-bold">${cost?.total_cost?.toFixed(4) || '0.0000'}</div>
            </div>
            <div className="p-3 bg-green-100 rounded-lg">
              <TrendingUp className="w-6 h-6 text-green-600" />
            </div>
          </div>
        </div>
      </div>

      {/* Quick Actions */}
      <div className="card mb-6">
        <div className="card-header">
          <h3 className="text-lg font-semibold">Quick Actions</h3>
        </div>
        <div className="card-body flex flex-wrap gap-3">
          <button className="btn btn-primary flex items-center gap-2">
            <Play className="w-4 h-4" />
            Run QA Test
          </button>
          <button className="btn btn-secondary flex items-center gap-2">
            <FileText className="w-4 h-4" />
            Generate Docs
          </button>
          <button 
            className="btn btn-secondary flex items-center gap-2"
            onClick={() => window.location.href = '/issues'}
          >
            <Bug className="w-4 h-4" />
            View All Issues
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Recent Activity */}
        <div className="card">
          <div className="card-header">
            <h3 className="text-lg font-semibold">Recent Activity</h3>
          </div>
          <div className="card-body">
            {recentActivity.length > 0 ? (
              <div className="space-y-3">
                {recentActivity.map((activity, idx) => (
                  <div key={idx} className="flex items-start gap-3 p-3 bg-gray-50 rounded-lg">
                    <div className={`p-2 rounded-lg ${
                      activity.type === 'flow' ? 'bg-blue-100' :
                      activity.type === 'issue' ? 'bg-red-100' : 'bg-green-100'
                    }`}>
                      {activity.type === 'flow' ? <Activity className="w-4 h-4 text-blue-600" /> :
                       activity.type === 'issue' ? <AlertTriangle className="w-4 h-4 text-red-600" /> :
                       <CheckCircle className="w-4 h-4 text-green-600" />}
                    </div>
                    <div className="flex-1">
                      <div className="font-medium">{activity.title}</div>
                      <div className="text-sm text-gray-500">{activity.description}</div>
                    </div>
                    <div className="text-xs text-gray-400">
                      <Clock className="w-3 h-3 inline mr-1" />
                      {new Date(activity.timestamp).toLocaleTimeString()}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-gray-500 text-center py-4">No recent activity</div>
            )}
          </div>
        </div>

        {/* Skills Summary */}
        <div className="card">
          <div className="card-header">
            <h3 className="text-lg font-semibold">Skills Performance</h3>
          </div>
          <div className="card-body">
            {skillStats.length > 0 ? (
              <div className="space-y-3">
                {skillStats.slice(0, 5).map((skill) => (
                  <div key={skill.id} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                    <div className="flex-1">
                      <div className="font-medium">{skill.name}</div>
                      <div className="text-sm text-gray-500">{skill.total_runs || 0} runs</div>
                    </div>
                    <div className="text-right">
                      <div className={`font-bold ${getPassRateColor(skill.pass_rate || 0)}`}>
                        {Math.round((skill.pass_rate || 0) * 100)}%
                      </div>
                      <div className="text-xs text-gray-400">
                        {skill.passed_runs || 0}/{skill.total_runs || 0} passed
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-gray-500 text-center py-4">No skill data available</div>
            )}
          </div>
        </div>
      </div>

      {/* Issue Summary */}
      <div className="mt-6 card">
        <div className="card-header">
          <h3 className="text-lg font-semibold">Issue Summary</h3>
        </div>
        <div className="card-body">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="p-4 bg-gray-50 rounded-lg">
              <div className="text-sm text-gray-500">Total Issues</div>
              <div className="text-2xl font-bold">{issueCounts.total}</div>
            </div>
            <div className="p-4 bg-red-50 rounded-lg">
              <div className="text-sm text-gray-500">Open</div>
              <div className="text-2xl font-bold text-red-600">{issueCounts.open}</div>
            </div>
            <div className="p-4 bg-green-50 rounded-lg">
              <div className="text-sm text-gray-500">Fixed</div>
              <div className="text-2xl font-bold text-green-600">{issueCounts.fixed}</div>
            </div>
          </div>
          
          {issueCounts.total > 0 && (
            <div className="mt-4">
              <div className="flex items-center gap-2 mb-2">
                <span className="text-sm text-gray-500">Fix Progress:</span>
                <span className="font-medium">
                  {Math.round((issueCounts.fixed / issueCounts.total) * 100)}%
                </span>
              </div>
              <div className="w-full bg-gray-200 rounded-full h-2">
                <div 
                  className="bg-green-600 h-2 rounded-full transition-all"
                  style={{ width: `${(issueCounts.fixed / issueCounts.total) * 100}%` }}
                ></div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}