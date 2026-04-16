import { useState, useEffect } from 'react';
import { 
  AlertTriangle, 
  FileCode, 
  CheckCircle, 
  XCircle,
  Search,
  Filter,
  Clock,
  ChevronRight,
  Lightbulb,
  Eye,
  Code
} from 'lucide-react';

interface Issue {
  id: number;
  page: string;
  action: string;
  error_type: string;
  error_text: string;
  category: string;
  severity: string;
  is_fixed: boolean;
  notes: string;
  created_at: string;
  suggestions: Suggestion[];
}

interface Suggestion {
  id: number;
  suggestion: string;
  confidence: number;
  target: string;
  file_path: string;
  diff: string;
}

export default function IssueDebugger() {
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<'all' | 'frontend' | 'backend'>('all');
  const [selectedIssue, setSelectedIssue] = useState<Issue | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [viewMode, setViewMode] = useState<'list' | 'detail'>('list');

  useEffect(() => {
    fetchIssues();
  }, [filter, searchQuery]);

  const fetchIssues = async () => {
    try {
      setLoading(true);
      let url = '/api/issues?limit=50';
      
      if (filter !== 'all') {
        url += `&target=${filter}`;
      }
      
      if (searchQuery) {
        url += `&search=${encodeURIComponent(searchQuery)}`;
      }

      const response = await fetch(url);
      const data = await response.json();
      
      if (data && data.data) {
        setIssues(data.data);
      } else if (Array.isArray(data)) {
        setIssues(data);
      } else {
        setIssues([]);
      }
    } catch (error) {
      console.error('Failed to fetch issues:', error);
      setIssues([]);
    } finally {
      setLoading(false);
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'high': return 'badge-error';
      case 'medium': return 'badge-warning';
      default: return 'badge-success';
    }
  };

  const getCategoryBadge = (category: string) => {
    return category === 'frontend' 
      ? 'bg-blue-100 text-blue-800' 
      : category === 'backend'
      ? 'bg-purple-100 text-purple-800'
      : 'bg-gray-100 text-gray-800';
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    try {
      return new Date(dateStr).toLocaleString();
    } catch {
      return dateStr;
    }
  };

  const handleSelectIssue = (issue: Issue) => {
    setSelectedIssue(issue);
    setViewMode('detail');
  };

  const handleBackToList = () => {
    setViewMode('list');
    setSelectedIssue(null);
  };

  if (loading && issues.length === 0) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900">Issue Debugger</h2>
        <p className="text-gray-600">Debug issues and view AI-generated fix suggestions</p>
      </div>

      {/* Search and Filters */}
      <div className="flex flex-col md:flex-row gap-4 mb-6">
        <div className="flex-1 relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
          <input
            type="text"
            placeholder="Search by page, action, or error text..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10 pr-4 py-2 w-full border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
          />
        </div>
        <div className="flex gap-2">
          {(['all', 'frontend', 'backend'] as const).map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`px-4 py-2 rounded-lg capitalize ${
                filter === f ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
              }`}
            >
              {f}
            </button>
          ))}
        </div>
      </div>

      {/* Stats Overview */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Issues</div>
          <div className="text-2xl font-bold">{issues.length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Frontend</div>
          <div className="text-2xl font-bold text-blue-600">
            {issues.filter(i => i.category === 'frontend').length}
          </div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Backend</div>
          <div className="text-2xl font-bold text-purple-600">
            {issues.filter(i => i.category === 'backend').length}
          </div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">With Suggestions</div>
          <div className="text-2xl font-bold text-green-600">
            {issues.filter(i => i.suggestions && i.suggestions.length > 0).length}
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Issues list */}
        <div className="lg:col-span-1 card">
          <div className="card-header">
            <div className="flex items-center justify-between">
              <h3 className="font-semibold">Issues</h3>
              <span className="text-sm text-gray-500">({issues.length})</span>
            </div>
          </div>
          <div className="divide-y divide-gray-200 max-h-[500px] overflow-y-auto">
            {issues.length === 0 ? (
              <div className="p-4 text-center text-gray-500">No issues found</div>
            ) : (
              issues.map((issue) => (
                <div
                  key={issue.id}
                  onClick={() => handleSelectIssue(issue)}
                  className={`p-4 cursor-pointer hover:bg-gray-50 transition-colors ${
                    selectedIssue?.id === issue.id ? 'bg-primary-50 border-l-4 border-primary-600' : ''
                  }`}
                >
                  <div className="flex items-start justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-gray-900 truncate">{issue.page}</div>
                      <div className="text-sm text-gray-500 truncate">{issue.action}</div>
                    </div>
                    <ChevronRight className="w-4 h-4 text-gray-400 flex-shrink-0 ml-2" />
                  </div>
                  {issue.error_text && (
                    <div className="mt-2 text-sm text-gray-600 truncate">
                      {issue.error_text}
                    </div>
                  )}
                  <div className="mt-2 flex items-center gap-2 flex-wrap">
                    <span className={`badge ${getSeverityColor(issue.severity)}`}>
                      {issue.severity}
                    </span>
                    <span className={`badge ${getCategoryBadge(issue.category)}`}>
                      {issue.category || 'unknown'}
                    </span>
                    {issue.is_fixed && (
                      <span className="badge badge-success">Fixed</span>
                    )}
                  </div>
                  <div className="mt-2 text-xs text-gray-400 flex items-center gap-1">
                    <Clock className="w-3 h-3" />
                    {formatDate(issue.created_at)}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        {/* Issue details */}
        <div className="lg:col-span-2 card">
          {selectedIssue ? (
            <>
              <div className="card-header">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <button 
                      onClick={handleBackToList}
                      className="p-1 hover:bg-gray-100 rounded"
                    >
                      <ChevronRight className="w-4 h-4 rotate-180" />
                    </button>
                    <h3 className="font-semibold">Issue #{selectedIssue.id}</h3>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className={`badge ${getSeverityColor(selectedIssue.severity)}`}>
                      {selectedIssue.severity}
                    </span>
                    {selectedIssue.is_fixed && (
                      <span className="badge badge-success">Fixed</span>
                    )}
                  </div>
                </div>
              </div>
              <div className="card-body space-y-4">
                {/* Basic Info */}
                <div>
                  <label className="text-sm text-gray-500">Page / Action</label>
                  <div className="font-medium text-gray-900">{selectedIssue.page}</div>
                  <div className="text-gray-600">{selectedIssue.action}</div>
                </div>

                {/* Error Type */}
                <div>
                  <label className="text-sm text-gray-500">Error Type</label>
                  <div className="font-medium text-gray-900">{selectedIssue.error_type || 'N/A'}</div>
                </div>

                {/* Error Text */}
                {selectedIssue.error_text && (
                  <div>
                    <label className="text-sm text-gray-500">Error Message</label>
                    <div className="p-3 bg-red-50 rounded-lg text-red-700 text-sm font-mono whitespace-pre-wrap break-words">
                      {selectedIssue.error_text}
                    </div>
                  </div>
                )}

                {/* Classification Info */}
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="text-sm text-gray-500">Category</label>
                    <div className={`inline-block px-2 py-1 rounded text-sm ${getCategoryBadge(selectedIssue.category)}`}>
                      {selectedIssue.category || 'unknown'}
                    </div>
                  </div>
                  <div>
                    <label className="text-sm text-gray-500">Created</label>
                    <div className="text-gray-900">{formatDate(selectedIssue.created_at)}</div>
                  </div>
                </div>

                {/* Suggestions */}
                <div>
                  <label className="text-sm text-gray-500 flex items-center gap-2">
                    <Lightbulb className="w-4 h-4" />
                    AI Suggestions
                  </label>
                  {selectedIssue.suggestions && selectedIssue.suggestions.length > 0 ? (
                    <div className="space-y-3">
                      {selectedIssue.suggestions.map((sug) => (
                        <div key={sug.id} className="p-3 bg-gray-50 rounded-lg">
                          <div className="flex items-center justify-between mb-2">
                            <div className="flex items-center gap-2">
                              <span className={`px-2 py-0.5 rounded text-xs ${
                                sug.target === 'frontend' ? 'bg-blue-100 text-blue-800' : 
                                sug.target === 'backend' ? 'bg-purple-100 text-purple-800' : 
                                'bg-gray-100 text-gray-800'
                              }`}>
                                {sug.target || 'unknown'}
                              </span>
                              {sug.file_path && (
                                <span className="text-xs text-gray-500 flex items-center gap-1">
                                  <Code className="w-3 h-3" />
                                  {sug.file_path}
                                </span>
                              )}
                            </div>
                            <div className="flex items-center gap-1">
                              <span className="text-xs text-gray-500">Confidence:</span>
                              <span className={`text-sm font-medium ${
                                sug.confidence >= 0.8 ? 'text-green-600' :
                                sug.confidence >= 0.5 ? 'text-yellow-600' : 'text-red-600'
                              }`}>
                                {Math.round(sug.confidence * 100)}%
                              </span>
                            </div>
                          </div>
                          <p className="text-gray-700">{sug.suggestion}</p>
                          
                          {sug.diff && (
                            <div className="mt-2">
                              <div className="text-xs text-gray-500 mb-1">Suggested Changes:</div>
                              <pre className="p-2 bg-slate-800 text-green-400 rounded text-xs overflow-x-auto">
                                {sug.diff}
                              </pre>
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="p-4 bg-gray-50 rounded-lg text-center text-gray-500">
                      No suggestions available for this issue
                    </div>
                  )}
                </div>

                {/* Notes */}
                {selectedIssue.notes && (
                  <div>
                    <label className="text-sm text-gray-500">Notes</label>
                    <div className="p-3 bg-yellow-50 rounded-lg text-yellow-800">
                      {selectedIssue.notes}
                    </div>
                  </div>
                )}

                {/* Actions */}
                <div className="flex gap-2 pt-2 border-t">
                  <button className="btn btn-primary flex items-center gap-2">
                    <FileCode className="w-4 h-4" />
                    Generate Patch
                  </button>
                  <button className="btn btn-secondary flex items-center gap-2">
                    <Eye className="w-4 h-4" />
                    View DOM
                  </button>
                </div>
              </div>
            </>
          ) : (
            <div className="card-body flex flex-col items-center justify-center h-96 text-gray-500">
              <AlertTriangle className="w-12 h-12 mb-4 text-gray-300" />
              <p>Select an issue from the list to view details</p>
              <p className="text-sm mt-2">Or use the search to find specific issues</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}