import { useState, useEffect } from 'react';
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell, BarChart, Bar, CartesianGrid, Legend } from 'recharts';

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

interface CostEfficiency {
  hit: number;
  miss: number;
}

const COLORS = ['#10b981', '#ef4444']; // Green for hit, Red for miss
const CHART_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6'];

export default function CostAnalytics() {
  const [overview, setOverview] = useState<CostOverview | null>(null);
  const [trends, setTrends] = useState<CostTrend[]>([]);
  const [byFeature, setByFeature] = useState<CostByFeature[]>([]);
  const [efficiency, setEfficiency] = useState<CostEfficiency>({ hit: 0, miss: 0 });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchData();
  }, []);

  const fetchData = async () => {
    try {
      const [overviewRes, trendRes, featureRes, efficiencyRes] = await Promise.all([
        fetch('/api/cost/overview'),
        fetch('/api/cost/trend'),
        fetch('/api/cost/by-feature'),
        fetch('/api/cost/efficiency'),
      ]);

      const overviewData = await overviewRes.json();
      const trendData = await trendRes.json();
      const featureData = await featureRes.json();
      const efficiencyData = await efficiencyRes.json();

      setOverview(overviewData);
      setTrends(trendData || []);
      setByFeature(featureData || []);
      setEfficiency(efficiencyData || { hit: 0, miss: 0 });
    } catch (error) {
      console.error('Failed to fetch cost data:', error);
    } finally {
      setLoading(false);
    }
  };

  // Calculate efficiency percentage
  const totalRequests = efficiency.hit + efficiency.miss;
  const hitPercentage = totalRequests > 0 ? (efficiency.hit / totalRequests * 100) : 0;
  const missPercentage = totalRequests > 0 ? (efficiency.miss / totalRequests * 100) : 0;

  // Prepare pie chart data
  const pieData = [
    { name: 'Cache Hits', value: efficiency.hit },
    { name: 'Cache Misses', value: efficiency.miss },
  ];

  // Prepare bar chart data for top 10 features
  const topFeatures = byFeature.slice(0, 10).map(item => ({
    feature: item.feature,
    cost: item.cost,
  }));

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900">Cost Analytics</h2>
        <p className="text-gray-600">AI usage costs and cache efficiency</p>
      </div>

      {/* Overview cards */}
      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          {Array.from({ length: 4 }).map((_, idx) => (
            <div key={idx} className="card p-4 animate-pulse">
              <div className="h-4 w-20 bg-gray-200 rounded mb-3" />
              <div className="h-8 w-24 bg-gray-100 rounded" />
            </div>
          ))}
        </div>
      ) : (
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
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
        <div className="card p-4">
          <div className="text-sm text-gray-500">Cache Hit Rate</div>
          <div className="flex items-center">
            <div className="flex-1 h-3 bg-gray-200 rounded-full overflow-hidden">
              <div 
                className={`h-full rounded-full ${hitPercentage > 50 ? 'bg-green-500' : 'bg-yellow-500'}`}
                style={{ width: `${hitPercentage}%` }}
              />
            </div>
            <span className="ml-3 font-medium">
              {hitPercentage.toFixed(1)}%
            </span>
          </div>
        </div>
      </div>
      )}

      {/* Charts row 1 */}
      {loading ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
          <div className="card h-64 animate-pulse bg-gray-50" />
          <div className="card h-64 animate-pulse bg-gray-50" />
        </div>
      ) : (
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        {/* Cost trend */}
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Daily Cost Trend (Last 30 Days)</h3>
          </div>
          <div className="card-body h-64">
            {trends.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={trends}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="date" tick={{ fontSize: 10 }} tickFormatter={(value) => value.slice(5)} />
                  <YAxis tick={{ fontSize: 10 }} tickFormatter={(value) => `$${value.toFixed(2)}`} />
                  <Tooltip formatter={(value: number) => [`$${value.toFixed(4)}`, 'Cost']} />
                  <Line 
                    type="monotone" 
                    dataKey="cost" 
                    stroke="#3b82f6" 
                    strokeWidth={2}
                    dot={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No trend data available
              </div>
            )}
          </div>
        </div>

        {/* Efficiency pie chart */}
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Cache Efficiency</h3>
          </div>
          <div className="card-body h-64">
            {totalRequests > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={pieData}
                    dataKey="value"
                    nameKey="name"
                    cx="50%"
                    cy="50%"
                    outerRadius={80}
                    label={({ name, percent }) => 
                      `${name}: ${(percent * 100).toFixed(1)}%`
                    }
                  >
                    {pieData.map((_, index) => (
                      <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                    ))}
                  </Pie>
                  <Tooltip formatter={(value: number) => [value, 'Requests']} />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No efficiency data available
              </div>
            )}
          </div>
        </div>
      </div>
      )}

      {/* Cost by feature - bar chart */}
      <div className="card">
        <div className="card-header">
          <h3 className="font-semibold">Cost by Feature (Top 10)</h3>
        </div>
        <div className="card-body h-80">
          {loading ? (
            <div className="h-full animate-pulse bg-gray-50 rounded" />
          ) : topFeatures.length > 0 ? (
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={topFeatures} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis type="number" tickFormatter={(value) => `$${value.toFixed(2)}`} />
                <YAxis type="category" dataKey="feature" width={120} tick={{ fontSize: 10 }} />
                <Tooltip formatter={(value: number) => [`$${value.toFixed(4)}`, 'Cost']} />
                <Bar dataKey="cost" fill="#3b82f6" />
              </BarChart>
            </ResponsiveContainer>
          ) : (
            <div className="flex items-center justify-center h-full text-gray-500">
              No feature data available
            </div>
          )}
        </div>
      </div>

      {/* Token breakdown */}
      <div className="mt-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500 mb-3">Token Breakdown</div>
          <div className="flex items-center gap-8">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-blue-500"></div>
              <span className="text-sm text-gray-600">Input Tokens:</span>
              <span className="font-medium">{(overview?.input_tokens || 0).toLocaleString()}</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 rounded-full bg-green-500"></div>
              <span className="text-sm text-gray-600">Output Tokens:</span>
              <span className="font-medium">{(overview?.output_tokens || 0).toLocaleString()}</span>
            </div>
            <div className="text-sm text-gray-500">
              (Ratio: {overview?.input_tokens && overview?.output_tokens 
                ? (overview.input_tokens / overview.output_tokens).toFixed(2) + ':1'
                : '0:0'})
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
