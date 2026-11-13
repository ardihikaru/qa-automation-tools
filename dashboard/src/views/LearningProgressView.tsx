import { useEffect, useMemo, useState } from 'react';
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  BarChart,
  Bar,
} from 'recharts';
import { Search, Brain, TrendingUp, BarChart3, Target } from 'lucide-react';

interface LearningRecord {
  id: number;
  intent: string;
  action: string;
  page: string;
  success_score: number;
  usage_count: number;
  last_success?: string | null;
  created_at?: string;
  updated_at?: string;
}

interface SuccessTrendPoint {
  date: string;
  success: number;
}

interface TopActionRow {
  key: string;
  intent: string;
  action: string;
  page: string;
  usage_count: number;
  avg_success: number;
}

export default function LearningProgressView() {
  const [records, setRecords] = useState<LearningRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [pageFilter, setPageFilter] = useState('all');

  useEffect(() => {
    fetchLearningRecords();
  }, []);

  const fetchLearningRecords = async () => {
    try {
      setLoading(true);
      setError(null);

      const res = await fetch('/api/learning?limit=500');
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }

      const data = await res.json();
      setRecords(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Failed to fetch learning records:', err);
      setError('Failed to load learning data');
      setRecords([]);
    } finally {
      setLoading(false);
    }
  };

  const filteredRecords = useMemo(() => {
    const q = search.trim().toLowerCase();

    return records.filter((record) => {
      const matchesSearch =
        q.length === 0 ||
        record.intent?.toLowerCase().includes(q) ||
        record.action?.toLowerCase().includes(q) ||
        record.page?.toLowerCase().includes(q);
      const matchesPage = pageFilter === 'all' || record.page === pageFilter;
      return matchesSearch && matchesPage;
    });
  }, [records, search, pageFilter]);

  const pages = useMemo(() => {
    const uniquePages = new Set<string>();
    records.forEach((record) => {
      if (record.page) uniquePages.add(record.page);
    });
    return Array.from(uniquePages).sort((a, b) => a.localeCompare(b));
  }, [records]);

  const usageStats = useMemo(() => {
    const totalRecords = filteredRecords.length;
    const totalUsage = filteredRecords.reduce((sum, r) => sum + (r.usage_count || 0), 0);
    const weightedSuccess =
      totalUsage > 0
        ? filteredRecords.reduce((sum, r) => sum + (r.success_score || 0) * (r.usage_count || 0), 0) /
          totalUsage
        : 0;
    const highSuccessActions = filteredRecords.filter((r) => (r.success_score || 0) >= 0.8).length;

    return {
      totalRecords,
      totalUsage,
      weightedSuccess,
      highSuccessActions,
    };
  }, [filteredRecords]);

  const successTrend = useMemo<SuccessTrendPoint[]>(() => {
    const buckets = new Map<string, { weightedSuccessSum: number; usageSum: number }>();

    filteredRecords.forEach((record) => {
      const refDate = record.last_success || record.updated_at || record.created_at;
      if (!refDate) return;

      const date = new Date(refDate);
      if (Number.isNaN(date.getTime())) return;
      const dateKey = date.toISOString().slice(0, 10);

      const usage = Math.max(1, record.usage_count || 0);
      const current = buckets.get(dateKey) || { weightedSuccessSum: 0, usageSum: 0 };
      current.weightedSuccessSum += (record.success_score || 0) * usage;
      current.usageSum += usage;
      buckets.set(dateKey, current);
    });

    return Array.from(buckets.entries())
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([date, value]) => ({
        date,
        success: value.usageSum > 0 ? value.weightedSuccessSum / value.usageSum : 0,
      }));
  }, [filteredRecords]);

  const topActions = useMemo<TopActionRow[]>(() => {
    const grouped = new Map<string, TopActionRow>();

    filteredRecords.forEach((record) => {
      const key = `${record.intent}::${record.action}::${record.page}`;
      const current =
        grouped.get(key) || {
          key,
          intent: record.intent || '-',
          action: record.action || '-',
          page: record.page || '-',
          usage_count: 0,
          avg_success: 0,
        };

      const usage = record.usage_count || 0;
      const prevUsage = current.usage_count;
      const totalUsage = prevUsage + usage;

      current.avg_success =
        totalUsage > 0
          ? (current.avg_success * prevUsage + (record.success_score || 0) * usage) / totalUsage
          : 0;
      current.usage_count = totalUsage;
      grouped.set(key, current);
    });

    return Array.from(grouped.values())
      .sort((a, b) => b.usage_count - a.usage_count)
      .slice(0, 10);
  }, [filteredRecords]);

  const topActionsChart = useMemo(
    () =>
      topActions.slice(0, 8).map((row) => ({
        name: row.action.length > 28 ? `${row.action.slice(0, 28)}...` : row.action,
        usage: row.usage_count,
      })),
    [topActions],
  );

  if (error) {
    return (
      <div>
        <div className="card p-6">
          <p className="text-red-600 mb-4">{error}</p>
          <button onClick={fetchLearningRecords} className="btn btn-primary">
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <p className="text-gray-600">Track learning cache quality, usage, and top actions</p>
      </div>

      <div className="flex flex-col md:flex-row gap-4 mb-6">
        <div className="flex-1 relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            placeholder="Search by intent, action, or page..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          />
        </div>
        <select
          value={pageFilter}
          onChange={(e) => setPageFilter(e.target.value)}
          className="px-4 py-2 border border-gray-300 rounded-lg"
        >
          <option value="all">All Pages</option>
          {pages.map((page) => (
            <option key={page} value={page}>
              {page}
            </option>
          ))}
        </select>
      </div>

      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          {Array.from({ length: 4 }).map((_, idx) => (
            <div key={idx} className="card p-4 animate-pulse">
              <div className="h-4 w-24 bg-gray-200 rounded mb-2" />
              <div className="h-8 w-20 bg-gray-100 rounded" />
            </div>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">Learning Records</div>
              <div className="text-2xl font-bold">{usageStats.totalRecords}</div>
            </div>
            <Brain className="w-6 h-6 text-blue-600" />
          </div>
        </div>
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">Total Usage</div>
              <div className="text-2xl font-bold">{usageStats.totalUsage.toLocaleString()}</div>
            </div>
            <BarChart3 className="w-6 h-6 text-green-600" />
          </div>
        </div>
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">Weighted Success</div>
              <div className="text-2xl font-bold">{Math.round(usageStats.weightedSuccess * 100)}%</div>
            </div>
            <TrendingUp className="w-6 h-6 text-primary-600" />
          </div>
        </div>
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-gray-500">High-Success Actions</div>
              <div className="text-2xl font-bold">{usageStats.highSuccessActions}</div>
            </div>
            <Target className="w-6 h-6 text-yellow-600" />
          </div>
        </div>
      </div>
      )}

      {loading ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
          <div className="card h-72 animate-pulse bg-gray-50" />
          <div className="card h-72 animate-pulse bg-gray-50" />
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Success Score Over Time</h3>
          </div>
          <div className="card-body h-72">
            {successTrend.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={successTrend}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="date" tickFormatter={(value) => value.slice(5)} tick={{ fontSize: 11 }} />
                  <YAxis domain={[0, 1]} tickFormatter={(value) => `${Math.round(value * 100)}%`} />
                  <Tooltip
                    formatter={(value: number) => `${Math.round(value * 100)}%`}
                    labelFormatter={(label) => `Date: ${label}`}
                  />
                  <Line type="monotone" dataKey="success" stroke="#2563eb" strokeWidth={2} dot={false} />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No trend data available for selected filters
              </div>
            )}
          </div>
        </div>

        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Top Actions by Usage</h3>
          </div>
          <div className="card-body h-72">
            {topActionsChart.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={topActionsChart} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis type="number" />
                  <YAxis dataKey="name" type="category" width={130} tick={{ fontSize: 11 }} />
                  <Tooltip formatter={(value: number) => [value, 'Usage']} />
                  <Bar dataKey="usage" fill="#10b981" />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No action data available for selected filters
              </div>
            )}
          </div>
        </div>
      </div>
      )}

      <div className="card">
        <div className="card-header">
          <h3 className="font-semibold">Top Learned Actions</h3>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Intent</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Action</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Page</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Usage</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Avg Success</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {loading ? (
                <tr>
                  <td colSpan={5} className="px-6 py-8 text-center text-gray-500">
                    Loading learned actions...
                  </td>
                </tr>
              ) : topActions.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-6 py-8 text-center text-gray-500">
                    No learned actions found
                  </td>
                </tr>
              ) : (
                topActions.map((row) => (
                  <tr key={row.key} className="hover:bg-gray-50">
                    <td className="px-6 py-4 text-gray-900">{row.intent}</td>
                    <td className="px-6 py-4 text-gray-900">{row.action}</td>
                    <td className="px-6 py-4 text-gray-600">{row.page}</td>
                    <td className="px-6 py-4 font-medium">{row.usage_count}</td>
                    <td className="px-6 py-4">
                      <span
                        className={`badge ${
                          row.avg_success >= 0.8
                            ? 'badge-success'
                            : row.avg_success >= 0.5
                              ? 'badge-warning'
                              : 'badge-error'
                        }`}
                      >
                        {Math.round(row.avg_success * 100)}%
                      </span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
