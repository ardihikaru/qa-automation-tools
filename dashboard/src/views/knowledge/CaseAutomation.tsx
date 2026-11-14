import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { FilePlus2, Play } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import { useToast } from '../../components/Toast';
import { CaseJob, createAutomation, errorText, getJobEvents, isTerminalJob, listCaseJobs, runCase } from './api';

export const JOB_CHIP_CLASS: Record<string, string> = {
  queued: 'bg-gray-100 text-gray-700',
  pending: 'bg-gray-100 text-gray-700',
  running: 'bg-blue-100 text-blue-800',
  completed: 'badge-success',
  failed: 'badge-error',
  skipped: 'bg-amber-100 text-amber-800',
};

/** A skipped case_run is a completed job: show it as its own chip. */
export const jobChipStatus = (status: string, outcome?: string) => (status === 'completed' && outcome === 'skipped' ? 'skipped' : status);

export const jobHref = (id: number | string) => `/jobs/${encodeURIComponent(String(id))}`;

export function JobChip({ job: j, pid, code }: { job: Pick<CaseJob, 'id' | 'status' | 'phase' | 'outcome'>; pid: number; code: string }) {
  const job = { ...j, status: jobChipStatus(j.status, j.outcome) };
  const active = !isTerminalJob(j.status);
  return (
    <Link
      to={jobHref(job.id)}
      state={{ pid, code }}
      title={`Job #${job.id}${job.phase ? `, phase ${job.phase}` : ''}`}
      aria-label={`Last job #${job.id}: ${job.status}${job.phase ? `, ${job.phase}` : ''}. Open`}
      className={`badge whitespace-nowrap hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${JOB_CHIP_CLASS[job.status] ?? 'bg-gray-100 text-gray-700'}`}
    >
      {active && <span className="inline-block w-1.5 h-1.5 rounded-full bg-current mr-1 motion-safe:animate-pulse" aria-hidden="true" />}
      {job.status}
      {active && job.phase ? ` · ${job.phase}` : ''}
    </Link>
  );
}

interface Props {
  pid: number;
  code: string;
  specPaths: string[] | null | undefined;
  /** The product has an automation repo (Create automation needs one). */
  hasRepo: boolean;
  /** The user may verify (same rule as the backend's canVerify). */
  canWrite: boolean;
  /** Table row: icon buttons, and the job history is not preloaded (one request per row). */
  compact?: boolean;
  removed?: boolean;
}

/** Run / Create automation buttons plus the status chip of the case's last job. */
export default function CaseAutomation({ pid, code, specPaths, hasRepo, canWrite, compact, removed }: Props) {
  const toast = useToast();
  const [last, setLast] = useState<CaseJob | null>(null);
  const [busy, setBusy] = useState<'run' | 'create' | null>(null);
  const hasSpec = !!specPaths?.length;

  useEffect(() => {
    setLast(null);
    if (compact) return;
    const ac = new AbortController();
    listCaseJobs(pid, code, ac.signal)
      .then((jobs) => !ac.signal.aborted && setLast(jobs[0] ?? null))
      
      .catch(() => undefined); // history is a nicety; the buttons still work
    return () => ac.abort();
  }, [pid, code, compact]);

  // Follow an active job so the chip and the disabled state clear without a reload.
  const activeId = last && !isTerminalJob(last.status) ? last.id : null;
  useEffect(() => {
    if (activeId === null) return;
    const ac = new AbortController();
    const h = window.setInterval(() => {
      getJobEvents(activeId, Number.MAX_SAFE_INTEGER, ac.signal)
        .then((r) => !ac.signal.aborted && r.status && setLast((cur) => (cur && cur.id === activeId ? { ...cur, status: r.status as string, phase: r.phase ?? cur.phase } : cur)))
        .catch(() => undefined);
    }, 3000);
    return () => {
      ac.abort();
      window.clearInterval(h);
    };
  }, [activeId]);

  const start = async (kind: 'run' | 'create') => {
    if (busy) return;
    setBusy(kind);
    try {
      const r = await (kind === 'run' ? runCase(pid, code) : createAutomation(pid, code));
      if (r?.job_id !== undefined) setLast({ id: r.job_id, status: 'queued', type: kind === 'run' ? 'case_run' : 'case_create' });
      toast.success(kind === 'run' ? `Run of ${code} queued.` : `Automation of ${code} queued.`);
    } catch (e) {
      toast.error(`${kind === 'run' ? 'Could not run' : 'Could not create automation for'} ${code}: ${errorText(e)}`);
    } finally {
      setBusy(null);
    }
  };

  const jobActive = activeId !== null;
  const canRun = canWrite && hasSpec && !removed;
  const canCreate = canWrite && !hasSpec && hasRepo && !removed;
  const reason = jobActive ? 'A job for this case is still active' : undefined;
  const size = compact ? 'h-7 px-2 text-xs' : 'h-8 px-3 text-sm';

  return (
    <span className={`inline-flex flex-wrap items-center gap-1.5 ${compact ? '' : 'gap-2'}`}>
      {canRun && (
        <SubmitButton
          loading={busy === 'run'}
          loadingLabel={compact ? <span className="sr-only">Starting run</span> : 'Starting…'}
          disabled={jobActive || busy !== null}
          title={reason ?? `Run ${code} on staging`}
          aria-label={`Run ${code}`}
          onClick={() => start('run')}
          className={`btn btn-secondary gap-1.5 ${size}`}
        >
          <Play className="w-3.5 h-3.5" aria-hidden="true" />
          Run
        </SubmitButton>
      )}
      {canCreate && (
        <SubmitButton
          loading={busy === 'create'}
          loadingLabel={compact ? <span className="sr-only">Starting</span> : 'Starting…'}
          disabled={jobActive || busy !== null}
          title={reason ?? `Generate a spec for ${code} and open a PR`}
          aria-label={`Create automation for ${code}`}
          onClick={() => start('create')}
          className={`btn btn-primary gap-1.5 ${size}`}
        >
          <FilePlus2 className="w-3.5 h-3.5" aria-hidden="true" />
          {compact ? 'Automate' : 'Create automation'}
        </SubmitButton>
      )}
      {last && <JobChip job={last} pid={pid} code={code} />}
    </span>
  );
}
