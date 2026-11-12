import { useEffect, useMemo, useState } from 'react';
import { CheckCircle, XCircle, Filter, Search, AlertCircle, ChevronLeft, ChevronRight } from 'lucide-react';
import {
  type FixTrackingAnalytics,
  type FixTrackingIssueRecord as Issue,
} from '../utils/fixTrackingMockGenerator';

interface IssuesPagination {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

interface IssuesResponse {
  data?: Issue[];
  pagination?: IssuesPagination;
  analytics?: FixTrackingAnalytics;
}

const DEFAULT_LIMIT = 20;
const EMPTY_ANALYTICS: FixTrackingAnalytics = {
  totalIssues: 0,
  fixedCount: 0,
  verifiedCount: 0,
  openCount: 0,
  inProgressCount: 0,
  fixRatePercentage: 0,
  averageTimeToResolveHours: 0,
  issuesBySeverity: {
    low: 0,
    medium: 0,
    high: 0,
    critical: 0,
  },
};

export default function FixTracking() {
  const [issues, setIssues] = useState<Issue[]>([]);
  const [analytics, setAnalytics] = useState<FixTrackingAnalytics>(EMPTY_ANALYTICS);
  const [loading, setLoading] = useState(true);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const [fixedFilter, setFixedFilter] = useState<'all' | 'fixed' | 'open'>('all');
  const [categoryFilter, setCategoryFilter] = useState<'all' | 'frontend' | 'backend' | 'unknown'>('all');
  const [searchInput, setSearchInput] = useState('');
  const [searchQuery, setSearchQuery] = useState('');
  const [sortBy, setSortBy] = useState<'date_newest' | 'date_oldest' | 'severity_high' | 'severity_low'>('date_newest');
  const [page, setPage] = useState(1);

  const [pagination, setPagination] = useState<IssuesPagination>({
    page: 1,
    limit: DEFAULT_LIMIT,
    total: 0,
    total_pages: 1,
  });

  const [selectedIssueId, setSelectedIssueId] = useState<number | null>(null);
  const [editingNotesIssueId, setEditingNotesIssueId] = useState<number | null>(null);
  const [notesText, setNotesText] = useState('');

  useEffect(() => {
    fetchIssues();
  }, [fixedFilter, categoryFilter, searchQuery, sortBy, page]);

  const applySearch = () => {
    setSearchQuery(searchInput.trim());
    setPage(1);
  };

  const fetchIssues = async () => {
    try {
      setLoading(true);
      setErrorMessage(null);

      const params = new URLSearchParams();
      params.set('limit', String(DEFAULT_LIMIT));
      params.set('page', String(page));
      params.set('sort_by', sortBy);

      if (fixedFilter === 'fixed') params.set('is_fixed', 'true');
      if (fixedFilter === 'open') params.set('is_fixed', 'false');
      if (categoryFilter !== 'all') params.set('category', categoryFilter);
      if (searchQuery) params.set('search', searchQuery);

      const response = await fetch(`/api/issues?${params.toString()}`);
      if (!response.ok) {
        throw new Error(`Request failed with status ${response.status}`);
      }

      const payload: IssuesResponse | Issue[] = await response.json();

      if (Array.isArray(payload)) {
        setIssues(payload);
        setPagination({
          page,
          limit: DEFAULT_LIMIT,
          total: payload.length,
          total_pages: 1,
        });
      } else {
        const data = Array.isArray(payload?.data) ? payload.data : [];
        const serverPagination = payload?.pagination;
        setAnalytics(payload?.analytics ?? EMPTY_ANALYTICS);

        setIssues(data);
        setPagination({
          page: serverPagination?.page ?? page,
          limit: serverPagination?.limit ?? DEFAULT_LIMIT,
          total: serverPagination?.total ?? data.length,
          total_pages: serverPagination?.total_pages ?? 1,
        });
      }
    } catch (error) {
      console.error('Failed to fetch issues:', error);
      setIssues([]);
      setAnalytics(EMPTY_ANALYTICS);
      setPagination({
        page: 1,
        limit: DEFAULT_LIMIT,
        total: 0,
        total_pages: 1,
      });
      setErrorMessage(error instanceof Error ? error.message : 'Unknown error when loading issues');
    } finally {
      setLoading(false);
    }
  };

  const selectedIssue = useMemo(
    () => issues.find((issue) => issue.id === selectedIssueId) ?? null,
    [issues, selectedIssueId]
  );

  useEffect(() => {
    if (!issues.length) {
      setSelectedIssueId(null);
      return;
    }

    if (selectedIssueId !== null && !issues.some((issue) => issue.id === selectedIssueId)) {
      setSelectedIssueId(null);
    }
  }, [issues, selectedIssueId]);

  const toggleFixed = async (issue: Issue) => {
    try {
      const updatedIsFixed = !issue.is_fixed;

      const response = await fetch('/api/issues/status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: issue.id,
          is_fixed: updatedIsFixed,
        }),
      });

      if (!response.ok) {
        throw new Error(`Failed to update status: ${response.status}`);
      }

      const payload = (await response.json()) as { status?: Issue['status'] };
      setIssues((prev) =>
        prev.map((i) => (i.id === issue.id ? { ...i, is_fixed: updatedIsFixed, status: payload.status || i.status } : i))
      );
      await fetchIssues();
    } catch (error) {
      console.error('Failed to update status:', error);
      setErrorMessage(error instanceof Error ? error.message : 'Failed to update issue status');
    }
  };

  const startEditNotes = (issue: Issue) => {
    setEditingNotesIssueId(issue.id);
    setNotesText(issue.notes || '');
  };

  const cancelEditNotes = () => {
    setEditingNotesIssueId(null);
    setNotesText('');
  };

  const saveNotes = async (issueId: number) => {
    try {
      const response = await fetch('/api/issues/notes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: issueId,
          notes: notesText,
        }),
      });

      if (!response.ok) {
        throw new Error(`Failed to save notes: ${response.status}`);
      }

      setIssues((prev) => prev.map((i) => (i.id === issueId ? { ...i, notes: notesText } : i)));
      cancelEditNotes();
      await fetchIssues();
    } catch (error) {
      console.error('Failed to save notes:', error);
      setErrorMessage(error instanceof Error ? error.message : 'Failed to save issue notes');
    }
  };

  const getSeverityColor = (severity: string) => {
    switch ((severity || '').toLowerCase()) {
      case 'high':
        return 'text-red-600';
      case 'medium':
        return 'text-yellow-600';
      default:
        return 'text-green-600';
    }
  };

  const formatDate = (dateValue: string) => {
    if (!dateValue) return 'N/A';
    try {
      return new Date(dateValue).toLocaleString();
    } catch {
      return dateValue;
    }
  };

  const fallbackText = (value: string | undefined, fallback: string) => {
    const v = (value || '').trim();
    return v ? v : fallback;
  };

  const isMalformedIssue = (issue: Issue) => {
    return !issue.page?.trim() || !issue.action?.trim() || !issue.severity?.trim();
  };

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900">Fix Tracking</h2>
        <p className="text-gray-600">Track issue resolution status with filters, details, and pagination</p>
      </div>

      {errorMessage && (
        <div className="mb-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 flex items-start gap-2">
          <AlertCircle className="w-4 h-4 mt-0.5" />
          <span>{errorMessage}</span>
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-6 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Issues</div>
          <div className="text-2xl font-bold">{analytics.totalIssues}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Fixed</div>
          <div className="text-2xl font-bold text-green-600">{analytics.fixedCount}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Verified</div>
          <div className="text-2xl font-bold text-blue-600">{analytics.verifiedCount}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Open</div>
          <div className="text-2xl font-bold text-red-600">{analytics.openCount}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Fix Rate</div>
          <div className="text-2xl font-bold">{analytics.fixRatePercentage}%</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Avg Resolve Time</div>
          <div className="text-2xl font-bold">{analytics.averageTimeToResolveHours}h</div>
        </div>
      </div>

      <div className="card mb-6 p-4">
        <div className="flex flex-col gap-4">
          <div className="flex items-center gap-2">
            <Filter className="w-5 h-5 text-gray-400" />
            <div className="flex flex-wrap gap-2">
              {(['all', 'open', 'fixed'] as const).map((f) => (
                <button
                  key={f}
                  onClick={() => {
                    setFixedFilter(f);
                    setPage(1);
                  }}
                  className={`px-4 py-2 rounded-lg capitalize ${
                    fixedFilter === f
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                  }`}
                >
                  {f}
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <select
              value={categoryFilter}
              onChange={(e) => {
                setCategoryFilter(e.target.value as 'all' | 'frontend' | 'backend' | 'unknown');
                setPage(1);
              }}
              className="px-3 py-2 border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
            >
              <option value="all">All Categories</option>
              <option value="frontend">Frontend</option>
              <option value="backend">Backend</option>
              <option value="unknown">Unknown</option>
            </select>

            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
              <input
                type="text"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') applySearch();
                }}
                placeholder="Search page, action, or error text"
                className="pl-10 pr-3 py-2 w-full border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
              />
            </div>

            <button onClick={applySearch} className="btn btn-primary">
              Search
            </button>

            <select
              value={sortBy}
              onChange={(e) => {
                setSortBy(
                  e.target.value as 'date_newest' | 'date_oldest' | 'severity_high' | 'severity_low'
                );
                setPage(1);
              }}
              className="px-3 py-2 border rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
            >
              <option value="date_newest">Date: Newest</option>
              <option value="date_oldest">Date: Oldest</option>
              <option value="severity_high">Severity: High First</option>
              <option value="severity_low">Severity: Low First</option>
            </select>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
        <div className="xl:col-span-2 card overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-gray-50 border-b">
                <tr>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">ID</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Page</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Action</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Error Type</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Category</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Severity</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Fixed</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Created</th>
                  <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase">Notes</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200">
                {loading ? (
                  <tr>
                    <td colSpan={9} className="px-6 py-8 text-center text-gray-500">
                      Loading issues...
                    </td>
                  </tr>
                ) : issues.length === 0 ? (
                  <tr>
                    <td colSpan={9} className="px-6 py-8 text-center text-gray-500">
                      No issues found
                    </td>
                  </tr>
                ) : (
                  issues.map((issue) => (
                    <tr
                      key={issue.id}
                      onClick={() => setSelectedIssueId(issue.id)}
                      className={`cursor-pointer hover:bg-gray-50 ${
                        selectedIssueId === issue.id ? 'bg-primary-50' : ''
                      }`}
                    >
                      <td className="px-4 py-4 text-sm font-medium text-gray-900">#{issue.id}</td>
                      <td className="px-4 py-4 text-sm text-gray-900 max-w-[180px] truncate">{fallbackText(issue.page, 'Unknown page')}</td>
                      <td className="px-4 py-4 text-sm text-gray-600 max-w-[160px] truncate">{fallbackText(issue.action, 'Unknown action')}</td>
                      <td className="px-4 py-4 text-sm text-gray-600 max-w-[160px] truncate">
                        {fallbackText(issue.error_type, 'N/A')}
                      </td>
                      <td className="px-4 py-4 text-sm">
                        <span className="badge bg-gray-100 text-gray-800 capitalize">
                          {(issue.category || 'unknown').toLowerCase()}
                        </span>
                        {isMalformedIssue(issue) && (
                          <span className="badge badge-warning ml-2">Legacy / Malformed</span>
                        )}
                      </td>
                      <td className="px-4 py-4 text-sm">
                        <span className={`font-medium capitalize ${getSeverityColor(issue.severity)}`}>
                          {fallbackText(issue.severity, 'Unknown severity')}
                        </span>
                      </td>
                      <td className="px-4 py-4" onClick={(e) => e.stopPropagation()}>
                        <button
                          onClick={() => toggleFixed(issue)}
                          className={`p-2 rounded-full ${
                            issue.is_fixed ? 'bg-green-100 text-green-600' : 'bg-gray-100 text-gray-400'
                          }`}
                          title={issue.is_fixed ? 'Mark as open' : 'Mark as fixed'}
                        >
                          {issue.is_fixed ? (
                            <CheckCircle className="w-4 h-4" />
                          ) : (
                            <XCircle className="w-4 h-4" />
                          )}
                        </button>
                      </td>
                      <td className="px-4 py-4 text-xs text-gray-500 whitespace-nowrap">
                        {formatDate(issue.created_at)}
                      </td>
                      <td className="px-4 py-4 text-sm" onClick={(e) => e.stopPropagation()}>
                        {editingNotesIssueId === issue.id ? (
                          <div className="flex flex-col gap-2 min-w-[220px]">
                            <textarea
                              value={notesText}
                              onChange={(e) => setNotesText(e.target.value)}
                              className="w-full px-2 py-1 border rounded text-sm"
                              rows={2}
                              placeholder="Add notes..."
                            />
                            <div className="flex gap-2">
                              <button
                                onClick={() => saveNotes(issue.id)}
                                className="text-xs px-2 py-1 rounded bg-primary-600 text-white"
                              >
                                Save
                              </button>
                              <button
                                onClick={cancelEditNotes}
                                className="text-xs px-2 py-1 rounded bg-gray-100 text-gray-700"
                              >
                                Cancel
                              </button>
                            </div>
                          </div>
                        ) : (
                          <button
                            onClick={() => startEditNotes(issue)}
                            className="text-left w-full text-gray-500 hover:text-gray-700"
                          >
                            {issue.notes || <span className="text-gray-400">Add notes...</span>}
                          </button>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          <div className="border-t bg-gray-50 px-4 py-3 flex items-center justify-between">
            <div className="text-sm text-gray-600">
              Showing page {pagination.page} of {Math.max(1, pagination.total_pages)} | Total: {pagination.total}
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                disabled={pagination.page <= 1 || loading}
                className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
              >
                <ChevronLeft className="w-4 h-4" />
                Prev
              </button>
              <button
                onClick={() => setPage((prev) => Math.min(pagination.total_pages || 1, prev + 1))}
                disabled={pagination.page >= pagination.total_pages || loading}
                className="px-3 py-1.5 rounded border bg-white text-gray-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
              >
                Next
                <ChevronRight className="w-4 h-4" />
              </button>
            </div>
          </div>
        </div>

        <div className="card p-4">
          <h3 className="text-lg font-semibold mb-4">Issue Detail</h3>
          {!selectedIssue ? (
            <div className="text-sm text-gray-500">Select an issue row to see full details.</div>
          ) : (
            <div className="space-y-4">
              <div>
                <div className="text-xs text-gray-500">Issue ID</div>
                <div className="font-medium text-gray-900">#{selectedIssue.id}</div>
              </div>
              <div>
                <div className="text-xs text-gray-500">Status</div>
                <div className="font-medium text-gray-900 capitalize">{selectedIssue.status || 'open'}</div>
              </div>

              <div>
                <div className="text-xs text-gray-500">Page / Action</div>
                <div className="font-medium text-gray-900 break-words">{fallbackText(selectedIssue.page, 'Unknown page')}</div>
                <div className="text-sm text-gray-600 break-words">{fallbackText(selectedIssue.action, 'Unknown action')}</div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-xs text-gray-500">Category</div>
                  <div className="text-sm font-medium capitalize">{selectedIssue.category || 'unknown'}</div>
                </div>
                <div>
                  <div className="text-xs text-gray-500">Severity</div>
                  <div className={`text-sm font-medium capitalize ${getSeverityColor(selectedIssue.severity)}`}>
                    {fallbackText(selectedIssue.severity, 'Unknown severity')}
                  </div>
                </div>
              </div>

              {isMalformedIssue(selectedIssue) && <div className="badge badge-warning">Legacy / Malformed issue row</div>}

              <div>
                <div className="text-xs text-gray-500">Error Type</div>
                <div className="text-sm text-gray-900">{fallbackText(selectedIssue.error_type, 'N/A')}</div>
              </div>

              <div>
                <div className="text-xs text-gray-500">Full Error Text</div>
                <pre className="mt-1 rounded-lg bg-red-50 text-red-700 p-3 text-xs whitespace-pre-wrap break-words">
                  {selectedIssue.error_text || 'No error text available'}
                </pre>
              </div>

              <div>
                <div className="text-xs text-gray-500">Created</div>
                <div className="text-sm text-gray-900">{formatDate(selectedIssue.created_at)}</div>
              </div>
              <div>
                <div className="text-xs text-gray-500">Resolved</div>
                <div className="text-sm text-gray-900">{formatDate(selectedIssue.resolved_at || '')}</div>
              </div>
              <div>
                <div className="text-xs text-gray-500">Fix Commit / PR</div>
                <div className="text-sm text-gray-900 break-all">
                  {selectedIssue.fix_commit_url ? (
                    <a
                      href={selectedIssue.fix_commit_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-blue-600 hover:underline"
                    >
                      Commit
                    </a>
                  ) : (
                    'N/A'
                  )}
                  {selectedIssue.fix_pr_url ? (
                    <>
                      {' '}
                      |{' '}
                      <a
                        href={selectedIssue.fix_pr_url}
                        target="_blank"
                        rel="noreferrer"
                        className="text-blue-600 hover:underline"
                      >
                        PR
                      </a>
                    </>
                  ) : null}
                </div>
              </div>

              <div>
                <div className="text-xs text-gray-500 mb-2">Suggestions</div>
                {selectedIssue.suggestions && selectedIssue.suggestions.length > 0 ? (
                  <div className="space-y-2">
                    {selectedIssue.suggestions.map((suggestion) => (
                      <div key={suggestion.id} className="rounded border bg-gray-50 p-2 text-xs">
                        <div className="font-medium text-gray-900 break-words">{suggestion.suggestion}</div>
                        <div className="mt-1 text-gray-600">
                          Target: {suggestion.target || 'unknown'}
                          {typeof suggestion.confidence === 'number'
                            ? ` | Confidence: ${(suggestion.confidence * 100).toFixed(0)}%`
                            : ''}
                        </div>
                        {suggestion.file_path && (
                          <div className="text-gray-500 break-words">File: {suggestion.file_path}</div>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="text-sm text-gray-500">No suggestions available.</div>
                )}
              </div>

              <div className="pt-1">
                <button
                  onClick={() => toggleFixed(selectedIssue)}
                  className={`px-3 py-2 rounded text-sm ${
                    selectedIssue.is_fixed
                      ? 'bg-green-100 text-green-700 hover:bg-green-200'
                      : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                  }`}
                >
                  {selectedIssue.is_fixed ? 'Mark as Open' : 'Mark as Fixed'}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
