import { useState, useEffect } from 'react';
import { CheckCircle, XCircle, Filter } from 'lucide-react';

interface Issue {
  id: number;
  page: string;
  action: string;
  category: string;
  severity: string;
  is_fixed: boolean;
  notes: string;
  created_at: string;
}

export default function FixTracking() {
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<'all' | 'fixed' | 'open'>('all');
  const [editingNotes, setEditingNotes] = useState<number | null>(null);
  const [notesText, setNotesText] = useState('');

  useEffect(() => {
    fetchIssues();
  }, [filter]);

  const fetchIssues = async () => {
    try {
      const isFixed = filter === 'fixed' ? 'true' : filter === 'open' ? 'false' : '';
      const url = isFixed ? `/api/issues?is_fixed=${isFixed}` : '/api/issues';
      const response = await fetch(url);
      const data = await response.json();
      setIssues(data || []);
    } catch (error) {
      console.error('Failed to fetch issues:', error);
    } finally {
      setLoading(false);
    }
  };

  const toggleFixed = async (issue: Issue) => {
    try {
      await fetch('/api/issues/status', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: issue.id,
          is_fixed: !issue.is_fixed,
        }),
      });
      
      // Update local state
      setIssues(issues.map(i => 
        i.id === issue.id ? { ...i, is_fixed: !i.is_fixed } : i
      ));
    } catch (error) {
      console.error('Failed to update status:', error);
    }
  };

  const saveNotes = async (issueId: number) => {
    try {
      await fetch('/api/issues/notes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: issueId,
          notes: notesText,
        }),
      });
      
      // Update local state
      setIssues(issues.map(i => 
        i.id === issueId ? { ...i, notes: notesText } : i
      ));
      
      setEditingNotes(null);
      setNotesText('');
    } catch (error) {
      console.error('Failed to save notes:', error);
    }
  };

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'high': return 'text-red-600';
      case 'medium': return 'text-yellow-600';
      default: return 'text-green-600';
    }
  };

  const fixedCount = issues.filter(i => i.is_fixed).length;
  const openCount = issues.filter(i => !i.is_fixed).length;
  const fixRate = issues.length > 0 ? Math.round((fixedCount / issues.length) * 100) : 0;

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
        <h2 className="text-2xl font-bold text-gray-900">Fix Tracking</h2>
        <p className="text-gray-600">Track issue resolution status</p>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="text-sm text-gray-500">Total Issues</div>
          <div className="text-2xl font-bold">{issues.length}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Fixed</div>
          <div className="text-2xl font-bold text-green-600">{fixedCount}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Open</div>
          <div className="text-2xl font-bold text-red-600">{openCount}</div>
        </div>
        <div className="card p-4">
          <div className="text-sm text-gray-500">Fix Rate</div>
          <div className="text-2xl font-bold">{fixRate}%</div>
        </div>
      </div>

      {/* Filter */}
      <div className="flex items-center gap-2 mb-6">
        <Filter className="w-5 h-5 text-gray-400" />
        {(['all', 'open', 'fixed'] as const).map((f) => (
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

      {/* Issues list */}
      <div className="card">
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead className="bg-gray-50 border-b">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Fixed</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Page</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Action</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Severity</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Category</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Notes</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {issues.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-6 py-8 text-center text-gray-500">
                    No issues found
                  </td>
                </tr>
              ) : (
                issues.map((issue) => (
                  <tr key={issue.id} className="hover:bg-gray-50">
                    <td className="px-6 py-4">
                      <button
                        onClick={() => toggleFixed(issue)}
                        className={`p-2 rounded-full ${
                          issue.is_fixed 
                            ? 'bg-green-100 text-green-600' 
                            : 'bg-gray-100 text-gray-400'
                        }`}
                      >
                        {issue.is_fixed ? (
                          <CheckCircle className="w-5 h-5" />
                        ) : (
                          <XCircle className="w-5 h-5" />
                        )}
                      </button>
                    </td>
                    <td className="px-6 py-4">
                      <div className="font-medium text-gray-900">{issue.page}</div>
                    </td>
                    <td className="px-6 py-4 text-gray-600">{issue.action}</td>
                    <td className="px-6 py-4">
                      <span className={`font-medium ${getSeverityColor(issue.severity)}`}>
                        {issue.severity}
                      </span>
                    </td>
                    <td className="px-6 py-4">
                      <span className="badge bg-gray-100 text-gray-800">
                        {issue.category || 'unknown'}
                      </span>
                    </td>
                    <td className="px-6 py-4">
                      {editingNotes === issue.id ? (
                        <div className="flex gap-2">
                          <input
                            type="text"
                            value={notesText}
                            onChange={(e) => setNotesText(e.target.value)}
                            className="flex-1 px-2 py-1 border rounded text-sm"
                            placeholder="Add notes..."
                          />
                          <button
                            onClick={() => saveNotes(issue.id)}
                            className="text-sm text-primary-600 hover:text-primary-700"
                          >
                            Save
                          </button>
                          <button
                            onClick={() => setEditingNotes(null)}
                            className="text-sm text-gray-500"
                          >
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <div 
                          onClick={() => {
                            setEditingNotes(issue.id);
                            setNotesText(issue.notes || '');
                          }}
                          className="cursor-pointer text-gray-500 hover:text-gray-700"
                        >
                          {issue.notes || <span className="text-gray-400">Add notes...</span>}
                        </div>
                      )}
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