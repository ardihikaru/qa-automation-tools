import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Search, ChevronRight, Lightbulb, Clock } from 'lucide-react';

interface Suggestion {
  id: number;
  suggestion: string;
  confidence: number;
  target: string;
  file_path: string;
  diff: string;
}

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

interface IssuesResponse {
  data?: Issue[];
}

function fallbackText(value: string | undefined, fallback: string) {
  const v = (value || '').trim();
  return v ? v : fallback;
}

function isMalformedIssue(issue: Issue) {
  return !issue.page?.trim() || !issue.action?.trim() || !issue.severity?.trim();
}

function IssueSkeleton() {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div className="lg:col-span-1 card p-4 animate-pulse">
        {Array.from({ length: 6 }).map((_, idx) => (
          <div key={idx} className="h-14 bg-gray-100 rounded mb-2" />
        ))}
      </div>
      <div className="lg:col-span-2 card p-4 animate-pulse">
        {Array.from({ length: 8 }).map((_, idx) => (
          <div key={idx} className="h-5 bg-gray-100 rounded mb-2" />
        ))}
      </div>
    </div>
  );
}

export default function IssueDebugger() {
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<'all' | 'frontend' | 'backend'>('all');
  const [selectedIssueId, setSelectedIssueId] = useState<number | null>(null);

  const [searchInput, setSearchInput] = useState('');
  const [searchQuery, setSearchQuery] = useState('');

  useEffect(() => {
    fetchIssues();
  }, [filter, searchQuery]);

  const fetchIssues = async () => {
    try {
      setLoading(true);
      let url = '/api/issues?limit=50';
      if (filter !== 'all') url += `&target=${filter}`;
      if (searchQuery) url += `&search=${encodeURIComponent(searchQuery)}`;

      const response = await fetch(url);
      const data: IssuesResponse | Issue[] = await response.json();

      if (Array.isArray(data)) {
        setIssues(data);
      } else if (Array.isArray(data?.data)) {
        setIssues(data.data);
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

  const selectedIssue = useMemo(() => issues.find((i) => i.id === selectedIssueId) || null, [issues, selectedIssueId]);

  const applySearch = () => {
    setSearchQuery(searchInput.trim());
  };

  const formatDate = (dateStr: string) => {
    if (!dateStr) return 'N/A';
    try {
      return new Date(dateStr).toLocaleString();
    } catch {
      return dateStr;
    }
  };

  return (
    <div>
      <div className="mb-6">
        <p className="text-gray-600">Debug issues and view AI-generated fix suggestions</p>
      </div>

      <div className="card mb-6 p-4">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
          <div className="md:col-span-2 relative">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') applySearch();
              }}
              placeholder="Search by page, action, or error text..."
              className="pl-10 pr-4 py-2 w-full border rounded-lg"
            />
          </div>
          <button className="btn btn-primary" onClick={applySearch}>Search</button>
          <div className="flex gap-2">
            {(['all', 'frontend', 'backend'] as const).map((f) => (
              <button
                key={f}
                onClick={() => setFilter(f)}
                className={`px-3 py-2 rounded-lg capitalize ${filter === f ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700'}`}
              >
                {f}
              </button>
            ))}
          </div>
        </div>
      </div>

      {loading ? (
        <IssueSkeleton />
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
            <div className="card p-4"><div className="text-sm text-gray-500">Total Issues</div><div className="text-2xl font-bold">{issues.length}</div></div>
            <div className="card p-4"><div className="text-sm text-gray-500">Frontend</div><div className="text-2xl font-bold text-blue-600">{issues.filter((i) => i.category === 'frontend').length}</div></div>
            <div className="card p-4"><div className="text-sm text-gray-500">Backend</div><div className="text-2xl font-bold text-purple-600">{issues.filter((i) => i.category === 'backend').length}</div></div>
            <div className="card p-4"><div className="text-sm text-gray-500">Malformed</div><div className="text-2xl font-bold text-amber-600">{issues.filter(isMalformedIssue).length}</div></div>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            <div className="lg:col-span-1 card overflow-hidden">
              <div className="card-header"><h3 className="font-semibold">Issues ({issues.length})</h3></div>
              <div className="divide-y divide-gray-200 max-h-[520px] overflow-y-auto">
                {issues.length === 0 ? (
                  <div className="p-6 text-center text-gray-500">No issues found</div>
                ) : (
                  issues.map((issue) => {
                    const malformed = isMalformedIssue(issue);
                    return (
                      <div
                        key={issue.id}
                        className={`p-4 cursor-pointer hover:bg-gray-50 ${selectedIssueId === issue.id ? 'bg-primary-50 border-l-4 border-primary-600' : ''}`}
                        onClick={() => setSelectedIssueId(issue.id)}
                      >
                        <div className="flex items-start justify-between">
                          <div className="min-w-0">
                            <div className="font-medium text-gray-900 truncate">{fallbackText(issue.page, 'Unknown page')}</div>
                            <div className="text-sm text-gray-500 truncate">{fallbackText(issue.action, 'Unknown action')}</div>
                          </div>
                          <ChevronRight className="w-4 h-4 text-gray-400 ml-2" />
                        </div>
                        <div className="mt-2 text-sm text-gray-600 truncate">{fallbackText(issue.error_text, 'No error text')}</div>
                        <div className="mt-2 flex items-center gap-2 flex-wrap">
                          <span className="badge capitalize">{fallbackText(issue.severity, 'Unknown severity')}</span>
                          <span className="badge capitalize">{fallbackText(issue.category, 'unknown')}</span>
                          {malformed && <span className="badge badge-warning">Legacy / Malformed</span>}
                        </div>
                        <div className="mt-2 text-xs text-gray-400 flex items-center gap-1">
                          <Clock className="w-3 h-3" /> {formatDate(issue.created_at)}
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </div>

            <div className="lg:col-span-2 card p-4">
              {!selectedIssue ? (
                <div className="h-full min-h-[420px] flex flex-col items-center justify-center text-gray-500">
                  <AlertTriangle className="w-10 h-10 text-gray-300 mb-3" />
                  Select an issue from the list to inspect details.
                </div>
              ) : (
                <div className="space-y-4">
                  <div>
                    <div className="text-xs text-gray-500">Page / Action</div>
                    <div className="font-medium text-gray-900">{fallbackText(selectedIssue.page, 'Unknown page')}</div>
                    <div className="text-gray-600">{fallbackText(selectedIssue.action, 'Unknown action')}</div>
                  </div>

                  <div className="grid grid-cols-2 gap-4">
                    <div>
                      <div className="text-xs text-gray-500">Severity</div>
                      <div className="text-sm font-medium">{fallbackText(selectedIssue.severity, 'Unknown severity')}</div>
                    </div>
                    <div>
                      <div className="text-xs text-gray-500">Category</div>
                      <div className="text-sm font-medium">{fallbackText(selectedIssue.category, 'unknown')}</div>
                    </div>
                  </div>

                  {isMalformedIssue(selectedIssue) && <div className="badge badge-warning">Legacy / Malformed issue row</div>}

                  <div>
                    <div className="text-xs text-gray-500">Error Text</div>
                    <pre className="p-3 bg-red-50 rounded text-red-700 text-xs whitespace-pre-wrap break-words">{fallbackText(selectedIssue.error_text, 'No error text')}</pre>
                  </div>

                  <div>
                    <div className="text-xs text-gray-500 flex items-center gap-1 mb-2"><Lightbulb className="w-4 h-4" />Suggestions</div>
                    {selectedIssue.suggestions && selectedIssue.suggestions.length > 0 ? (
                      <div className="space-y-2">
                        {selectedIssue.suggestions.map((s) => (
                          <div key={s.id} className="p-3 bg-gray-50 rounded">
                            <div className="text-sm text-gray-900">{s.suggestion}</div>
                            <div className="text-xs text-gray-600 mt-1">Target: {fallbackText(s.target, 'unknown')} | Confidence: {Math.round((s.confidence || 0) * 100)}%</div>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="text-sm text-gray-500">No suggestions available.</div>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
