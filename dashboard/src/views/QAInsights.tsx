import { useEffect, useMemo, useState } from 'react';
import { Activity, AlertTriangle, CheckCircle, Play, FileText, Bug, Clock, TrendingUp, RefreshCcw } from 'lucide-react';

interface AnalyticsData {
  flows: number;
  issues: number;
  skills: number;
}

interface CostOverview {
  total_cost: number;
}

interface IssueCount {
  total: number;
  open: number;
  fixed: number;
}

type JobStatus = 'idle' | 'queued' | 'running' | 'completed' | 'failed';

type JobType = 'qa_run' | 'docs_generate';

interface JobRecord {
  id: string;
  type: JobType;
  status: JobStatus;
  payload?: Record<string, unknown>;
  result?: Record<string, unknown>;
  error?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
}

function QAInsightsSkeleton() {
  return (
    <>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        {Array.from({ length: 4 }).map((_, idx) => (
          <div key={idx} className="card p-4 animate-pulse">
            <div className="h-4 w-20 bg-gray-200 rounded mb-3" />
            <div className="h-8 w-24 bg-gray-100 rounded" />
          </div>
        ))}
      </div>
      <div className="card p-4 animate-pulse mb-6">
        <div className="h-5 w-36 bg-gray-200 rounded mb-4" />
        <div className="h-10 w-full bg-gray-100 rounded" />
      </div>
    </>
  );
}

export default function QAInsights() {
  const [analytics, setAnalytics] = useState<AnalyticsData | null>(null);
  const [cost, setCost] = useState<CostOverview | null>(null);
  const [issueCounts, setIssueCounts] = useState<IssueCount>({ total: 0, open: 0, fixed: 0 });
  const [loading, setLoading] = useState(true);

  const [qaJob, setQaJob] = useState<JobRecord | null>(null);
  const [docsJob, setDocsJob] = useState<JobRecord | null>(null);
  const [qaActionStatus, setQaActionStatus] = useState<JobStatus>('idle');
  const [docsActionStatus, setDocsActionStatus] = useState<JobStatus>('idle');
  const [jobError, setJobError] = useState<string | null>(null);
  const [jobAudit, setJobAudit] = useState<JobRecord[]>([]);

  useEffect(() => {
    fetchAllData();
  }, []);

  const fetchAllData = async () => {
    try {
      setLoading(true);
      const [analyticsRes, costRes, allIssuesRes, openRes, fixedRes] = await Promise.all([
        fetch('/api/analytics'),
        fetch('/api/cost/overview'),
        fetch('/api/issues?limit=1'),
        fetch('/api/issues?is_fixed=false&limit=1'),
        fetch('/api/issues?is_fixed=true&limit=1'),
      ]);

      const analyticsData = await analyticsRes.json();
      const costData = await costRes.json();
      const allIssuesData = await allIssuesRes.json();
      const openData = await openRes.json();
      const fixedData = await fixedRes.json();

      setAnalytics(analyticsData);
      setCost(costData);
      setIssueCounts({
        total: allIssuesData?.pagination?.total || 0,
        open: openData?.pagination?.total || 0,
        fixed: fixedData?.pagination?.total || 0,
      });
    } catch (error) {
      console.error('Failed to fetch QA insights:', error);
    } finally {
      setLoading(false);
    }
  };

  const appendAudit = (job: JobRecord) => {
    setJobAudit((prev) => {
      const next = [job, ...prev.filter((x) => x.id !== job.id)];
      return next.slice(0, 12);
    });
  };

  const pollJob = async (jobId: string, onUpdate: (job: JobRecord) => void) => {
    const started = Date.now();
    while (Date.now() - started < 60000) {
      const res = await fetch(`/api/jobs/${encodeURIComponent(jobId)}`);
      if (!res.ok) {
        throw new Error(`Failed to poll job ${jobId} (${res.status})`);
      }
      const payload: JobRecord = await res.json();
      onUpdate(payload);
      appendAudit(payload);

      if (payload.status === 'completed' || payload.status === 'failed') {
        return payload;
      }
      await new Promise((resolve) => setTimeout(resolve, 800));
    }
    throw new Error('Job polling timeout');
  };

  const runJob = async (type: JobType) => {
    try {
      setJobError(null);
      if (type === 'qa_run') setQaActionStatus('queued');
      if (type === 'docs_generate') setDocsActionStatus('queued');

      const route = type === 'qa_run' ? '/api/jobs/qa/run' : '/api/jobs/docs/generate';
      const res = await fetch(route, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ triggered_by: 'dashboard', source: 'qa_insights' }),
      });

      if (!res.ok) {
        throw new Error(`Failed to start ${type} job (${res.status})`);
      }

      const created: JobRecord = await res.json();
      appendAudit(created);

      if (type === 'qa_run') {
        setQaJob(created);
        setQaActionStatus(created.status);
      } else {
        setDocsJob(created);
        setDocsActionStatus(created.status);
      }

      const finalJob = await pollJob(created.id, (job) => {
        if (type === 'qa_run') {
          setQaJob(job);
          setQaActionStatus(job.status);
        } else {
          setDocsJob(job);
          setDocsActionStatus(job.status);
        }
      });

      if (finalJob.status === 'failed') {
        setJobError(finalJob.error || 'Job failed');
      }
    } catch (error) {
      console.error('Failed to run job:', error);
      setJobError(error instanceof Error ? error.message : 'Failed to run job');
      if (type === 'qa_run') setQaActionStatus('failed');
      if (type === 'docs_generate') setDocsActionStatus('failed');
    }
  };

  const retryJob = (job: JobRecord) => {
    runJob(job.type);
  };

  const progress = useMemo(() => {
    if (issueCounts.total === 0) return 0;
    return Math.round((issueCounts.fixed / issueCounts.total) * 100);
  }, [issueCounts]);

  const statusBadge = (status: JobStatus) => {
    switch (status) {
      case 'completed':
        return 'badge-success';
      case 'failed':
        return 'badge-error';
      case 'running':
      case 'queued':
        return 'badge-warning';
      default:
        return 'badge';
    }
  };

  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900">QA Insights</h2>
        <p className="text-gray-600">Overview of QA automation activities and system health</p>
      </div>

      {loading ? (
        <QAInsightsSkeleton />
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
            <div className="card p-4">
              <div className="text-sm text-gray-500">Total Flows</div>
              <div className="text-2xl font-bold">{analytics?.flows || 0}</div>
            </div>
            <div className="card p-4">
              <div className="text-sm text-gray-500">Open Issues</div>
              <div className="text-2xl font-bold text-red-600">{issueCounts.open}</div>
            </div>
            <div className="card p-4">
              <div className="text-sm text-gray-500">Active Skills</div>
              <div className="text-2xl font-bold">{analytics?.skills || 0}</div>
            </div>
            <div className="card p-4">
              <div className="text-sm text-gray-500">Total Cost</div>
              <div className="text-2xl font-bold text-green-600">${(cost?.total_cost || 0).toFixed(4)}</div>
            </div>
          </div>

          <div className="card mb-6">
            <div className="card-header">
              <h3 className="text-lg font-semibold">Quick Actions</h3>
            </div>
            <div className="card-body flex flex-wrap gap-3 items-center">
              <button
                className="btn btn-primary flex items-center gap-2"
                disabled={qaActionStatus === 'queued' || qaActionStatus === 'running'}
                onClick={() => runJob('qa_run')}
              >
                <Play className="w-4 h-4" />
                {qaActionStatus === 'queued' || qaActionStatus === 'running' ? 'Running QA...' : 'Run QA Test'}
              </button>

              <button
                className="btn btn-secondary flex items-center gap-2"
                disabled={docsActionStatus === 'queued' || docsActionStatus === 'running'}
                onClick={() => runJob('docs_generate')}
              >
                <FileText className="w-4 h-4" />
                {docsActionStatus === 'queued' || docsActionStatus === 'running' ? 'Generating...' : 'Generate Docs'}
              </button>

              <button className="btn btn-secondary flex items-center gap-2" onClick={() => (window.location.href = '/issues')}>
                <Bug className="w-4 h-4" /> View All Issues
              </button>

              {qaJob && <span className={`badge ${statusBadge(qaJob.status)}`}>QA: {qaJob.status}</span>}
              {docsJob && <span className={`badge ${statusBadge(docsJob.status)}`}>Docs: {docsJob.status}</span>}
            </div>
            {jobError && <div className="px-6 pb-4 text-sm text-red-600">{jobError}</div>}
          </div>

          <div className="card mb-6">
            <div className="card-header">
              <h3 className="text-lg font-semibold">Action Audit</h3>
            </div>
            <div className="card-body overflow-x-auto">
              {jobAudit.length === 0 ? (
                <div className="text-gray-500 text-sm">No quick-action jobs executed yet.</div>
              ) : (
                <table className="w-full">
                  <thead className="bg-gray-50 border-b">
                    <tr>
                      <th className="px-4 py-2 text-left text-xs text-gray-500 uppercase">Type</th>
                      <th className="px-4 py-2 text-left text-xs text-gray-500 uppercase">Status</th>
                      <th className="px-4 py-2 text-left text-xs text-gray-500 uppercase">Started</th>
                      <th className="px-4 py-2 text-left text-xs text-gray-500 uppercase">Duration</th>
                      <th className="px-4 py-2 text-left text-xs text-gray-500 uppercase">Action</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {jobAudit.map((job) => (
                      <tr key={job.id}>
                        <td className="px-4 py-3 text-sm text-gray-900">{job.type}</td>
                        <td className="px-4 py-3 text-sm"><span className={`badge ${statusBadge(job.status)}`}>{job.status}</span></td>
                        <td className="px-4 py-3 text-sm text-gray-600">{job.started_at ? new Date(job.started_at).toLocaleString() : '-'}</td>
                        <td className="px-4 py-3 text-sm text-gray-600">{typeof job.duration_ms === 'number' ? `${job.duration_ms} ms` : '-'}</td>
                        <td className="px-4 py-3 text-sm">
                          {job.status === 'failed' ? (
                            <button className="btn btn-secondary px-2 py-1 text-xs flex items-center gap-1" onClick={() => retryJob(job)}>
                              <RefreshCcw className="w-3 h-3" /> Retry
                            </button>
                          ) : (
                            <span className="text-gray-400">-</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <div className="card p-4">
              <h3 className="text-lg font-semibold mb-3">Issue Summary</h3>
              <div className="grid grid-cols-3 gap-3 mb-3">
                <div>
                  <div className="text-sm text-gray-500">Total</div>
                  <div className="text-xl font-bold">{issueCounts.total}</div>
                </div>
                <div>
                  <div className="text-sm text-gray-500">Open</div>
                  <div className="text-xl font-bold text-red-600">{issueCounts.open}</div>
                </div>
                <div>
                  <div className="text-sm text-gray-500">Fixed</div>
                  <div className="text-xl font-bold text-green-600">{issueCounts.fixed}</div>
                </div>
              </div>
              <div className="w-full bg-gray-200 rounded-full h-2">
                <div className="bg-green-600 h-2 rounded-full" style={{ width: `${progress}%` }} />
              </div>
              <div className="text-sm text-gray-500 mt-2">Fix progress: {progress}%</div>
            </div>

            <div className="card p-4">
              <h3 className="text-lg font-semibold mb-3">Recent Health Signals</h3>
              <div className="space-y-2 text-sm text-gray-700">
                <div className="flex items-center gap-2"><Activity className="w-4 h-4 text-blue-600" /> Flows tracked: {analytics?.flows || 0}</div>
                <div className="flex items-center gap-2"><AlertTriangle className="w-4 h-4 text-red-600" /> Open issues: {issueCounts.open}</div>
                <div className="flex items-center gap-2"><CheckCircle className="w-4 h-4 text-green-600" /> Fixed issues: {issueCounts.fixed}</div>
                <div className="flex items-center gap-2"><Clock className="w-4 h-4 text-gray-600" /> Last job: {jobAudit[0]?.type || 'N/A'}</div>
                <div className="flex items-center gap-2"><TrendingUp className="w-4 h-4 text-green-600" /> Cost: ${(cost?.total_cost || 0).toFixed(4)}</div>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
