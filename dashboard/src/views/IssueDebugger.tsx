import { useState, useEffect } from 'react';
import { AlertTriangle, FileCode, CheckCircle, XCircle } from 'lucide-react';

interface Issue {
  id: number;
  page: string;
  action: string;
  error_type: string;
  error_text: string;
  category: string;
  severity: string;
  is_fixed: boolean;
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

  useEffect(() => {
    fetchIssues();
  }, [filter]);

  const fetchIssues = async () => {
    try {
      const url = filter === 'all' ? '/api/issues' : `/api/issues?target=${filter}`;
      const response = await fetch(url);
      const data = await response.json();
      setIssues(data || []);
    } catch (error) {
      console.error('Failed to fetch issues:', error);
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
        <h2 className="text-2xl font-bold text-gray-900">Issue Debugger</h2>
        <p className="text-gray-600">AI-generated suggestions and code patches</p>
      </div>

      {/* Filters */}
      <div className="flex gap-2 mb-6">
        {(['all', 'frontend', 'backend'] as const).map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={`px-4 py-2 rounded-lg capitalize ${
              filter === f ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'
            }`}
          >
            {f}
          </button>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Issues list */}
        <div className="lg:col-span-1 card">
          <div className="card-header">
            <h3 className="font-semibold">Issues ({issues.length})</h3>
          </div>
          <div className="divide-y divide-gray-200 max-h-96 overflow-y-auto">
            {issues.length === 0 ? (
              <div className="p-4 text-center text-gray-500">No issues found</div>
            ) : (
              issues.map((issue) => (
                <div
                  key={issue.id}
                  onClick={() => setSelectedIssue(issue)}
                  className={`p-4 cursor-pointer hover:bg-gray-50 ${
                    selectedIssue?.id === issue.id ? 'bg-primary-50 border-l-4 border-primary-600' : ''
                  }`}
                >
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="font-medium text-gray-900">{issue.page}</div>
                      <div className="text-sm text-gray-500">{issue.action}</div>
                    </div>
                    <span className={`badge ${getSeverityColor(issue.severity)}`}>
                      {issue.severity}
                    </span>
                  </div>
                  {issue.error_text && (
                    <div className="mt-2 text-sm text-gray-600 truncate">
                      {issue.error_text}
                    </div>
                  )}
                  <div className="mt-2 flex items-center gap-2">
                    <span className={`badge ${getCategoryBadge(issue.category)}`}>
                      {issue.category || 'unknown'}
                    </span>
                    {issue.is_fixed && (
                      <span className="badge badge-success">Fixed</span>
                    )}
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
                  <h3 className="font-semibold">Issue Details</h3>
                  <span className={`badge ${getSeverityColor(selectedIssue.severity)}`}>
                    {selectedIssue.severity}
                  </span>
                </div>
              </div>
              <div className="card-body">
                <div className="mb-4">
                  <label className="text-sm text-gray-500">Page / Action</label>
                  <div className="font-medium">{selectedIssue.page} / {selectedIssue.action}</div>
                </div>
                
                {selectedIssue.error_text && (
                  <div className="mb-4">
                    <label className="text-sm text-gray-500">Error</label>
                    <div className="p-3 bg-red-50 rounded-lg text-red-700 text-sm font-mono">
                      {selectedIssue.error_text}
                    </div>
                  </div>
                )}

                {/* Suggestions */}
                <div className="mb-4">
                  <label className="text-sm text-gray-500">Suggestions</label>
                  {selectedIssue.suggestions?.length > 0 ? (
                    <div className="space-y-2">
                      {selectedIssue.suggestions.map((sug) => (
                        <div key={sug.id} className="p-3 bg-gray-50 rounded-lg">
                          <div className="flex items-center justify-between mb-2">
                            <span className={`badge ${sug.target === 'frontend' ? 'bg-blue-100 text-blue-800' : 'bg-purple-100 text-purple-800'}`}>
                              {sug.target}
                            </span>
                            <span className="text-sm text-gray-500">
                              Confidence: {Math.round(sug.confidence * 100)}%
                            </span>
                          </div>
                          <p className="text-gray-700">{sug.suggestion}</p>
                          {sug.file_path && (
                            <div className="mt-2 text-sm text-gray-500">
                              File: {sug.file_path}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="text-gray-500">No suggestions available</div>
                  )}
                </div>

                {/* Actions */}
                <div className="flex gap-2">
                  <button className="btn btn-primary flex items-center gap-2">
                    <FileCode className="w-4 h-4" />
                    View Patch
                  </button>
                </div>
              </div>
            </>
          ) : (
            <div className="card-body flex items-center justify-center h-64 text-gray-500">
              Select an issue to view details
            </div>
          )}
        </div>
      </div>
    </div>
  );
}