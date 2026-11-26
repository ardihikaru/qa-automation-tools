// Runs of one test case, newest first. Shared by the test case page ("Run history") and the
// job page ("Other runs of this TC", which hides the job being viewed).
import { Link } from 'react-router-dom';
import { Check, X } from 'lucide-react';
import { CaseJobRow, listCaseJobHistory } from './api';
import { jobChipStatus, jobHref, JOB_CHIP_CLASS } from './CaseAutomation';
import { useLoad } from './hooks';
import { ErrorNote } from './ui';
import { formatInZone, useSystemTimezone } from '../../lib/time';

const isFail = (j: CaseJobRow) => j.status === 'failed' || j.outcome === 'fail';
const isOk = (j: CaseJobRow) => j.status === 'completed' && !isFail(j) && j.outcome !== 'skipped';

function RunIcon({ job }: { job: CaseJobRow }) {
  if (isFail(job)) return <X className="w-4 h-4 text-red-600 shrink-0" aria-label="failed" />;
  if (isOk(job)) return <Check className="w-4 h-4 text-green-600 shrink-0" aria-label="passed" />;
  const s = jobChipStatus(job.status, job.outcome ?? undefined);
  return <span className={`badge whitespace-nowrap ${JOB_CHIP_CLASS[s] ?? 'bg-gray-100 text-gray-700'}`}>{s}</span>;
}

interface Props {
  pid: number;
  code: string;
  /** Hide this job id (the one being viewed). */
  excludeId?: number | string;
  limit?: number;
  /** Bump to reload. */
  reloadKey?: number;
  /** Shows the pass/fail counts next to the title area. */
  showSummary?: boolean;
  emptyText?: string;
}

export default function JobHistory({ pid, code, excludeId, limit = 10, reloadKey = 0, showSummary = true, emptyText = 'No runs yet.' }: Props) {
  const tz = useSystemTimezone();
  const { data, loading, error, reload } = useLoad(`${pid}:${code}:${limit}:${reloadKey}`, (s) => listCaseJobHistory(pid, code, limit, s));
  const rows = (data?.jobs ?? []).filter((j) => excludeId === undefined || String(j.id) !== String(excludeId));
  const sum = data?.summary;

  return (
    <div className="space-y-2 min-w-0">
      {showSummary && sum && (
        <p className="text-sm text-gray-700 tabular-nums" aria-label={`${sum.success} passed, ${sum.failed} failed`}>
          <span className="text-green-700">✓ {sum.success}</span> · <span className="text-red-700">✗ {sum.failed}</span>
        </p>
      )}
      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}
      {loading && !data && <p className="text-sm text-gray-500">Loading runs…</p>}
      {data && rows.length === 0 && <p className="text-sm text-gray-500">{emptyText}</p>}
      {rows.length > 0 && (
        <ul className="rounded-lg border border-gray-200 bg-white divide-y divide-gray-100 overflow-hidden" aria-busy={loading}>
          {rows.map((j) => (
            <li key={j.id} className="flex items-center gap-2 px-3 py-1.5 text-sm min-w-0">
              <RunIcon job={j} />
              <time dateTime={j.created_at ?? undefined} className="text-xs text-gray-600 tabular-nums whitespace-nowrap">
                {formatInZone(j.started_at ?? j.created_at, tz) || '—'}
              </time>
              {j.type && <span className="text-xs text-gray-400 truncate">{j.type}</span>}
              {isFail(j) && j.error && (
                <span className="text-xs text-red-700 truncate min-w-0" title={j.error}>
                  {j.error}
                </span>
              )}
              <Link to={jobHref(j.id)} state={{ pid, code }} className="ml-auto shrink-0 font-mono text-xs text-primary-700 hover:underline min-h-6">
                #{j.id}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
