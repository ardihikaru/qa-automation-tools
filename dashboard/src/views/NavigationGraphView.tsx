import { useState, useEffect } from 'react';
import { 
  GitBranch, 
  Search, 
  MapPin, 
  ArrowRight, 
  Circle,
  ZoomIn,
  ZoomOut,
  RotateCcw,
  ChevronRight,
  ChevronDown,
  Info,
  Target,
  FlowChart
} from 'lucide-react';

interface NavigationEdge {
  id: number;
  source_page: string;
  target_page: string;
  action: string;
  success_count: number;
  failure_count: number;
  success_rate: number;
  last_used?: string;
}

interface NavigationNode {
  id: string;
  label: string;
  type: 'page' | 'action' | 'goal';
  x?: number;
  y?: number;
}

export default function NavigationGraphView() {
  const [edges, setEdges] = useState<NavigationEdge[]>([]);
  const [nodes, setNodes] = useState<NavigationNode[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchSource, setSearchSource] = useState('');
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'list' | 'graph'>('list');
  const [expandedSource, setExpandedSource] = useState<string | null>(null);

  useEffect(() => {
    fetchNavigationData();
  }, []);

  const fetchNavigationData = async () => {
    try {
      setLoading(true);
      
      // Get learning navigation data
      const learningRes = await fetch('/api/learning?limit=100');
      const learningData = await learningRes.json();
      
      if (Array.isArray(learningData) {
        // Create unique nodes from the learning data
        const uniquePages = new Set<string>();
        const nodeList: NavigationNode[] = [];
        
        learningData.forEach((item: any) => {
          if (item.page) {
            uniquePages.add(item.page);
            nodeList.push({ id: item.page, label: item.page, type: 'page' });
          }
        });
        
        setNodes(nodeList);
        setEdges(learningData.map((item: any, idx: number) => ({
          id: idx,
          source_page: item.page || 'unknown',
          target_page: item.action || 'unknown',
          action: item.action || 'navigate',
          success_count: Math.round(item.usage_count * item.success_score),
          failure_count: Math.round(item.usage_count * (1 - item.success_score)),
          success_rate: item.success_score || 0.5,
          last_used: item.last_success,
        })));
      }
    } catch (error) {
      console.error('Failed to fetch navigation data:', error);
    } finally {
      setLoading(false);
    }
  };

  const fetchSourceEdges = async (sourcePage: string) => {
    try {
      const res = await fetch(`/api/learning/navigation?source=${encodeURIComponent(sourcePage)}`);
      const data = await res.json();
      return Array.isArray(data) ? data : [];
    } catch (error) {
      console.error('Failed to fetch source edges:', error);
      return [];
    }
  };

  const handleSearch = async () => {
    if (!searchSource.trim()) {
      fetchNavigationData();
      return;
    }
    
    try {
      setLoading(true);
      const res = await fetch(`/api/learning/navigation?source=${encodeURIComponent(searchSource)}`);
      const data = await res.json();
      
      if (Array.isArray(data)) {
        setEdges(data.map((item: any, idx: number) => ({
          id: idx,
          source_page: item.source_page || searchSource,
          target_page: item.target_page || 'unknown',
          action: item.action || 'click',
          success_count: item.success_count || 0,
          failure_count: item.failure_count || 0,
          success_rate: item.success_rate || 0.5,
        })));
      }
    } catch (error) {
      console.error('Search failed:', error);
    } finally {
      setLoading(false);
    }
  };

  const getUniqueSources = () => {
    const sources = new Set(edges.map(e => e.source_page));
    return Array.from(sources);
  };

  const getEdgesFromSource = (source: string) => {
    return edges.filter(e => e.source_page === source);
  };

  const getSuccessRateColor = (rate: number) => {
    if (rate >= 0.8) return 'text-green-600';
    if (rate >= 0.5) return 'text-yellow-600';
    return 'text-red-600';
  };

  const getSuccessRateBg = (rate: number) => {
    if (rate >= 0.8) return 'bg-green-100';
    if (rate >= 0.5) return 'bg-yellow-100';
    return 'bg-red-100';
  };

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return 'Never';
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
        <h2 className="text-2xl font-bold text-gray-900">Navigation Graph</h2>
        <p className="text-gray-600">Visualize page navigation paths and connections</p>
      </div>

      {/* Controls */}
      <div className="flex flex-col md:flex-row gap-4 mb-6">
        <div className="flex-1 relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            placeholder="Search by source page..."
            value={searchSource}
            onChange={(e) => setSearchSource(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          />
        </div>
        
        <div className="flex gap-2">
          <button
            onClick={() => setViewMode('list')}
            className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
              viewMode === 'list' 
                ? 'bg-primary-600 text-white' 
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            <GitBranch className="w-4 h-4" />
            List View
          </button>
          <button
            onClick={() => setViewMode('graph')}
            className={`px-4 py-2 rounded-lg flex items-center gap-2 ${
              viewMode === 'graph' 
                ? 'bg-primary-600 text-white' 
                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }`}
          >
            <FlowChart className="w-4 h-4" />
            Graph View
          </button>
        </div>
      </div>

      {/* Stats Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Pages</div>
          <div className="text-2xl font-bold">{nodes.length || getUniqueSources().length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Navigation Edges</div>
          <div className="text-2xl font-bold">{edges.length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Successful Paths</div>
          <div className="text-2xl font-bold text-green-600">
            {edges.filter(e => e.success_rate >= 0.5).length}
          </div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Avg Success Rate</div>
          <div className="text-2xl font-bold">
            {edges.length > 0 
              ? Math.round(edges.reduce((a, e) => a + e.success_rate, 0) / edges.length * 100)
              : 0}%
          </div>
        </div>
      </div>

      {/* List View */}
      {viewMode === 'list' && (
        <div className="space-y-4">
          {getUniqueSources().length === 0 ? (
            <div className="card p-8 text-center text-gray-500">
              <MapPin className="w-12 h-12 mx-auto mb-4 text-gray-300" />
              <p>No navigation data available</p>
              <p className="text-sm mt-2">Run QA explorations to generate navigation data</p>
            </div>
          ) : (
            getUniqueSources().map(source => (
              <div key={source} className="card">
                <div 
                  className="card-header cursor-pointer flex items-center justify-between"
                  onClick={() => setExpandedSource(expandedSource === source ? null : source)}
                >
                  <div className="flex items-center gap-2">
                    <MapPin className="w-5 h-5 text-primary-600" />
                    <h3 className="font-semibold">{source}</h3>
                    <span className="badge">
                      {getEdgesFromSource(source).length} connections
                    </span>
                  </div>
                  {expandedSource === source ? (
                    <ChevronDown className="w-5 h-5 text-gray-400" />
                  ) : (
                    <ChevronRight className="w-5 h-5 text-gray-400" />
                  )}
                </div>
                
                {expandedSource === source && (
                  <div className="card-body divide-y divide-gray-200">
                    {getEdgesFromSource(source).map((edge) => (
                      <div key={edge.id} className="py-3 flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <ArrowRight className="w-4 h-4 text-gray-400" />
                          <div>
                            <div className="font-medium text-gray-900">
                              {edge.target_page}
                            </div>
                            <div className="text-sm text-gray-500">
                              via {edge.action}
                            </div>
                          </div>
                        </div>
                        <div className="text-right">
                          <div className={`font-bold ${getSuccessRateColor(edge.success_rate)}`}>
                            {Math.round(edge.success_rate * 100)}%
                          </div>
                          <div className="text-xs text-gray-500">
                            {edge.success_count} success, {edge.failure_count} failed
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      )}

      {/* Graph View */}
      {viewMode === 'graph' && (
        <div className="card">
          <div className="card-header">
            <h3 className="font-semibold">Visual Graph</h3>
          </div>
          <div className="card-body">
            {edges.length === 0 ? (
              <div className="text-center py-12 text-gray-500">
                <FlowChart className="w-12 h-12 mx-auto mb-4 text-gray-300" />
                <p>No graph data to display</p>
                <p className="text-sm mt-2">Search for a specific page to see its connections</p>
              </div>
            ) : (
              <div className="relative overflow-x-auto">
                <div className="min-w-[600px] py-8">
                  {getUniqueSources().map((source, sourceIdx) => (
                    <div key={source} className="flex items-start mb-8">
                      {/* Source Node */}
                      <div className="flex-shrink-0 w-40">
                        <div className={`p-3 rounded-lg border-2 ${
                          selectedNode === source 
                            ? 'border-primary-600 bg-primary-50' 
                            : 'border-primary-300 bg-primary-100'
                        }`}>
                          <MapPin className="w-4 h-4 inline mr-1 text-primary-600" />
                          <span className="font-medium text-sm">{source}</span>
                        </div>
                      </div>
                      
                      {/* Arrow */}
                      <div className="flex items-center justify-center w-12">
                        <ArrowRight className="w-6 h-6 text-gray-400" />
                      </div>
                      
                      {/* Target Nodes */}
                      <div className="flex-1 flex flex-wrap gap-2">
                        {getEdgesFromSource(source).slice(0, 5).map((edge, idx) => (
                          <div 
                            key={edge.id}
                            onClick={() => setSelectedNode(edge.target_page)}
                            className={`p-2 rounded-lg border cursor-pointer transition-colors ${
                              selectedNode === edge.target_page
                                ? 'border-green-600 bg-green-50'
                                : `border-2 ${getSuccessRateBg(edge.success_rate)}`
                            }`}
                          >
                            <Circle className={`w-2 h-2 inline mr-1 ${
                              edge.success_rate >= 0.8 ? 'text-green-600' :
                              edge.success_rate >= 0.5 ? 'text-yellow-600' : 'text-red-600'
                            }`} />
                            <span className="text-sm">{edge.target_page}</span>
                          </div>
                        ))}
                        {getEdgesFromSource(source).length > 5 && (
                          <div className="p-2 text-sm text-gray-500">
                            +{getEdgesFromSource(source).length - 5} more
                          </div>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
            
            {/* Legend */}
            <div className="mt-6 pt-4 border-t flex items-center gap-6">
              <div className="text-sm text-gray-500">Legend:</div>
              <div className="flex items-center gap-2">
                <Circle className="w-2 h-2 text-green-600" />
                <span className="text-sm">High (≥80%)</span>
              </div>
              <div className="flex items-center gap-2">
                <Circle className="w-2 h-2 text-yellow-600" />
                <span className="text-sm">Medium (50-79%)</span>
              </div>
              <div className="flex items-center gap-2">
                <Circle className="w-2 h-2 text-red-600" />
                <span className="text-sm">Low (&lt;50%)</span>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}