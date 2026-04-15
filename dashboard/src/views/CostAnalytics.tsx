import { useState, useEffect } from 'react';
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts';

interface CostOverview {
  total_requests: number;
  total_tokens: number;
  input_tokens: number;
  output_tokens: number;
  total_cost: number;
  cache_hit_rate: number;
}

interface CostTrend {
  date: string;
  cost: number;
}

interface CostByFeature {
  feature: string;
  cost: number;
}

const COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6'];

export default function CostAnalytics() {
  const [overview, setOverview] = useState<CostOverview | null>(null);
  const [trend, setTrend] = useState<CostTrend[]>([]);
  const [byFeature, setByFeature] = useState<CostByFeature[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      const [overviewRes, trendRes, featureRes] = await Promise.all([
        fetch('/api/cost/overview'),
        fetch('/api/cost/trend'),
        fetch('/api/cost/by-feature'),
      ]);

      const overviewData = await overviewRes.json();
      const trendData = await trendRes.json();
      const featureData = await featureRes.json();

      setOverview(overviewData);
      setTrend(trendData || []);
      setByFeature(featureData || []);
    } catch (error) {
      console.error('Failed to fetch cost data:', error);
    } finally {
      setLoading(false);
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
        <h2 className="text-2xl font-bold text-gray-900">Cost Analytics</h2>
        <p className="text-gray-600">AI usage costs and cache efficiency</p>
      </div>

      {/* Overview cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Requests</div>
          <div className="text-2xl font-bold">{overview?.total_requests || 0}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Tokens</div>
          <div className="text-2xl font-bold">{(overview?.total_tokens || 0).toLocaleString()}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Cost</div>
          <div className="text-2xl font-bold text-green-600">
            ${(overview?.total_cost || 0).toFixed(4)}
          </div>
        </div>
      </div>

      {/* Cache efficiency */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-2">Cache Hit Rate</div>
          <div className="flex items-center">
            <div className="flex-1 h-3 bg-gray-200 rounded-full overflow-hidden">
              <div 
                className="h-full bg-green-500 rounded-full"
                style={{ width: `${overview?.cache_hit_rate || 0}%` }}
              />
            </div>
            <span className="ml-3 font-medium">
              {Math.round(overview?.cache_hit_rate || 0)}%
            </span>
          </div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-2">Token Breakdown</div>
          <div className="flex items-center gap-4 text-sm">
            <div>
              <span className="text-gray-500">Input:</span>{' '}
              <span className="font-medium">{(overview?.input_tokens || 0).toLocaleString()}</span>
            </div>
            <div>
              <span className="text-gray-500">Output:</span>{' '}
              <span className="font-medium">{(overview?.output_tokens || 0).toLocaleString()}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Cost trend */}
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Daily Cost Trend</h3>
          </div>
          <div className="card-body h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={trend}>
                <XAxis dataKey="date" tick={{ fontSize: 12 }} />
                <YAxis tick={{ fontSize: 12 }} />
                <Tooltip />
                <Line 
                  type="monotone" 
                  dataKey="cost" 
                  stroke="#3b82f6" 
                  strokeWidth={2}
                  dot={false}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>

        {/* Cost by feature */}
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Cost by Feature</h3>
          </div>
          <div className="card-body h-64">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={byFeature}
                  dataKey="cost"
                  nameKey="feature"
                  cx="50%"
                  cy="50%"
                  outerRadius={80}
                  label={({ feature, percent }) => 
                    `${feature} (${(percent * 100).toFixed(0)}%)`
                  }
                >
                  {byFeature.map((_, index) => (
                    <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>
    </div>
  );
}