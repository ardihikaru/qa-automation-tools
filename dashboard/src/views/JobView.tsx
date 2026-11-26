import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import { ArrowDown, ArrowLeft, Check, ExternalLink, GitBranch, GitPullRequest, MessageSquareWarning, X } from 'lucide-react';
import SubmitButton from '../components/SubmitButton';
import { useToast } from '../components/Toast';
import { usePageHeader } from '../components/pageHeader';
import {
  ApiError,
  ApprovalDecision,
  JobApproval,
  JobEvent,
  JobEvents,
  JobReport,
  answerApproval,
  entityHref,
  errorText,
  getJob,
  getJobEvents,
  JobInfo,
  isAbort,
  isTerminalJob,
} from './knowledge/api';
import { JOB_CHIP_CLASS, jobChipStatus } from './knowledge/CaseAutomation';
import JobConversationCard from './conversations/JobConversationCard';
import { answerSourceLabel } from './history/api';
import JobHistory from './knowledge/JobHistory';
import { formatElapsed, formatInZone, useSystemTimezone } from '../lib/time';

const POLL_MS = 2000;
const AFTER_TERMINAL_MS = 30_000;
const MAX_EVENTS = 2000;

const LEVEL_CLASS: Record<string, string> = {
  error: 'text-red-700 bg-red-50',
  warn: 'text-amber-800 bg-amber-50',
  warning: 'text-amber-800 bg-amber-50',
  info: 'text-gray-800',
  debug: 'text-gray-500',
};
const STEP_CLASS = 'bg-gray-100 text-gray-700';

const fmtTime = (at?: string) => {
  if (!at) return '';
  const d = new Date(at);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour12: false });
};

const safeUrl = (u?: string) => (u && /^https?:\/\//i.test(u) ? u : null);

/** Report counts from the structured field, else from "N passed / N failed / N skipped" in the run events. */
function reportFrom(r: JobReport | null | undefined, events: JobEvent[]): JobReport | null {
  if (r && (r.passed !== undefined || r.failed !== undefined || r.skipped !== undefined)) return r;
  for (let i = events.length - 1; i >= 0; i--) {
    const m = events[i].message ?? '';
    if (!/\b(passed|failed|skipped)\b/i.test(m) || !/\d/.test(m)) continue;
    const num = (w: string) => {
      const x = new RegExp(`(\\d+)\\s+${w}`, 'i').exec(m);
      return x ? Number(x[1]) : undefined;
    };
    const out = { passed: num('passed'), failed: num('failed'), skipped: num('skipped') };
    if (out.passed !== undefined || out.failed !== undefined || out.skipped !== undefined) return out;
  }
  return null;
}

const SPEC_PATH_RE = /[\w./-]+\.spec\.ts/;

export default function JobView() {
  const { id = '' } = useParams();
  const loc = useLocation();
  const hint = (loc.state ?? {}) as { pid?: number; code?: string };
  const toast = useToast();

  const [events, setEvents] = useState<JobEvent[]>([]);
  const [meta, setMeta] = useState<Omit<JobEvents, 'events'>>({});
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fatal, setFatal] = useState(false);
  const [settledAt, setSettledAt] = useState<number | null>(null);
  const [poll, setPoll] = useState(0); // bump to restart polling (after an answer)

  const seqRef = useRef(0);
  const terminalSince = useRef<number | null>(null);

  // Reset when the job changes.
  useEffect(() => {
    seqRef.current = 0;
    terminalSince.current = null;
    setEvents([]);
    setMeta({});
    setLoaded(false);
    setError(null);
    setFatal(false);
    setSettledAt(null);
  }, [id]);

  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    let timer: number | undefined;
    terminalSince.current = null;
    setSettledAt(null);

    const tick = async () => {
      try {
        const r = await getJobEvents(id, seqRef.current, ac.signal);
        if (ac.signal.aborted) return;
        const fresh = r.events.filter((e) => typeof e?.seq === 'number' && e.seq > seqRef.current);
        if (fresh.length) {
          seqRef.current = Math.max(seqRef.current, ...fresh.map((e) => e.seq));
          setEvents((cur) => [...cur, ...fresh].slice(-MAX_EVENTS));
        }
        const { events: _drop, ...rest } = r;
        void _drop;
        setMeta((cur) => ({ ...cur, ...rest }));
        setError(null);
        setLoaded(true);
        if (isTerminalJob(r.status)) {
          if (terminalSince.current === null) terminalSince.current = Date.now();
          if (Date.now() - terminalSince.current >= AFTER_TERMINAL_MS) {
            setSettledAt(Date.now());
            return; // stop polling
          }
        } else {
          terminalSince.current = null;
        }
      } catch (e) {
        if (ac.signal.aborted || isAbort(e)) return;
        if (e instanceof ApiError && (e.status === 404 || e.status === 403)) {
          setError(e.status === 404 ? 'This job does not exist.' : 'You are not allowed to see this job.');
          setFatal(true);
          setLoaded(true);
          return;
        }
        setError(errorText(e, 'Could not load the job'));
      }
      timer = window.setTimeout(tick, POLL_MS);
    };
    tick();
    return () => {
      ac.abort();
      window.clearTimeout(timer);
    };
  }, [id, poll]);

  const tz = useSystemTimezone();
  const [info, setInfo] = useState<JobInfo | null>(null);
  const [now, setNow] = useState(() => Date.now());

  // Timing and case link from GET /api/jobs/{id}; reloaded when the status changes.
  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    getJob(id, ac.signal)
      .then((j) => !ac.signal.aborted && setInfo(j))
      .catch(() => undefined);
    return () => ac.abort();
  }, [id, meta.status]);
  useEffect(() => setInfo(null), [id]);

  const code = info?.case_code ?? meta.case_code ?? hint.code;
  const pid = info?.product_id ?? meta.product_id ?? hint.pid;
  const startedAt = info?.started_at ?? null;
  const endedAt = info?.completed_at ?? null;
  const running = !isTerminalJob(meta.status ?? info?.status) && !!(info?.started_at ?? info?.created_at);
  useEffect(() => {
    if (!running) return;
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, [running]);
  const startMs = Date.parse(info?.started_at ?? info?.created_at ?? '');
  const elapsedMs = running && !Number.isNaN(startMs) ? Math.max(0, now - startMs) : (info?.elapsed_ms ?? info?.duration_ms ?? null);
  const status = jobChipStatus(meta.status ?? (loaded ? 'unknown' : 'loading'), meta.outcome);
  const terminal = isTerminalJob(meta.status);
  const caseHref = pid !== undefined && code ? entityHref(pid, 'test_case', code) : null;

  usePageHeader({
    title: `Job #${id}`,
    crumbs: [{ label: 'Products', to: '/products' }, ...(caseHref && code ? [{ label: code, to: caseHref }] : [])],
  });

  const spec = useMemo(() => {
    if (meta.spec?.content || meta.spec?.path) return meta.spec;
    const w = [...events].reverse().find((e) => e.step === 'write');
    const path = w?.message ? SPEC_PATH_RE.exec(w.message)?.[0] : undefined;
    return path ? { path, content: undefined } : null;
  }, [meta.spec, events]);
  const report = useMemo(() => reportFrom(meta.report, events), [meta.report, events]);
  const prUrl = safeUrl(meta.pr_url);

  if (fatal) {
    return (
      <div className="space-y-3">
        <p role="alert" className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">
          {error}
        </p>
        <Link to="/products" className="inline-flex items-center gap-1.5 text-sm text-primary-700 hover:underline">
          <ArrowLeft className="w-4 h-4" aria-hidden="true" /> Back to products
        </Link>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <header className="card">
        <div className="card-body flex flex-wrap items-center gap-x-4 gap-y-2 min-w-0">
          {caseHref && (
            <Link to={caseHref} aria-label="Back to the test case" className="inline-flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 min-h-8">
              <ArrowLeft className="w-4 h-4" aria-hidden="true" />
            </Link>
          )}
          <h1 className="text-lg font-semibold text-gray-900">Job #{id}</h1>
          <span className={`badge whitespace-nowrap ${JOB_CHIP_CLASS[status] ?? 'bg-gray-100 text-gray-700'}`} role="status">
            {status}
          </span>
          {meta.outcome === 'skipped' && (
            <span className="text-sm text-amber-800 min-w-0 break-words">
              Skipped{meta.skip_reason ? `: ${meta.skip_reason}` : ' (no reason in the report)'}. No result was stored.
            </span>
          )}
          {meta.phase && (
            <span className="text-sm text-gray-600">
              Phase <span className="font-mono text-gray-900">{meta.phase}</span>
            </span>
          )}
          {code && (
            <span className="text-sm text-gray-600 min-w-0">
              Case{' '}
              {caseHref ? (
                <Link to={caseHref} className="font-mono text-primary-700 hover:underline break-all">
                  {code}
                </Link>
              ) : (
                <span className="font-mono text-gray-900 break-all">{code}</span>
              )}
            </span>
          )}
          {startedAt && (
            <span className="text-sm text-gray-600">
              Started <time dateTime={startedAt} className="text-gray-900 tabular-nums">{formatInZone(startedAt, tz)}</time>
            </span>
          )}
          {endedAt && (
            <span className="text-sm text-gray-600">
              Ended <time dateTime={endedAt} className="text-gray-900 tabular-nums">{formatInZone(endedAt, tz)}</time>
            </span>
          )}
          {elapsedMs !== null && formatElapsed(elapsedMs) && (
            <span className="text-sm text-gray-600">
              Elapsed <span className="text-gray-900 tabular-nums">{formatElapsed(elapsedMs)}</span>
            </span>
          )}
          {meta.branch && (
            <span className="inline-flex items-center gap-1 text-sm text-gray-600 min-w-0">
              <GitBranch className="w-4 h-4 shrink-0" aria-hidden="true" />
              <span className="font-mono text-gray-900 break-all">{meta.branch}</span>
            </span>
          )}
          {prUrl && (
            <a href={prUrl} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1.5 text-sm font-medium text-primary-700 hover:underline min-h-8 min-w-0">
              <GitPullRequest className="w-4 h-4 shrink-0" aria-hidden="true" />
              <span className="truncate">Pull request</span>
              <ExternalLink className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
              <span className="sr-only">(opens in a new tab)</span>
            </a>
          )}
        </div>
      </header>

      {error && (
        <p role="alert" className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-2.5 text-sm text-amber-900">
          {error} Retrying every {POLL_MS / 1000} s.
        </p>
      )}

      {meta.approval && <ApprovalCard jobId={id} approval={meta.approval} terminal={terminal} onAnswered={() => setPoll((n) => n + 1)} toast={toast} />}

      <div className="grid gap-4 xl:grid-cols-3 items-start">
        <section aria-labelledby="job-timeline" className="card xl:col-span-2 min-w-0">
          <div className="card-header flex items-center justify-between gap-2">
            <h2 id="job-timeline" className="font-medium text-gray-900">
              Timeline
            </h2>
            <span className="text-xs text-gray-500" aria-live="polite">
              {!terminal ? 'Live, refreshing every 2 s' : settledAt ? 'Finished' : 'Finished, checking for late events'}
            </span>
          </div>
          <Timeline events={events} loaded={loaded} terminal={terminal} />
        </section>

        <div className="space-y-4 min-w-0">
          <section aria-labelledby="job-report" className="card">
            <div className="card-header">
              <h2 id="job-report" className="font-medium text-gray-900">
                Report
              </h2>
            </div>
            <div className="card-body">
              {report ? (
                <dl className="grid grid-cols-3 gap-2 text-center">
                  <Stat label="Passed" value={report.passed} cls="text-green-700" />
                  <Stat label="Failed" value={report.failed} cls="text-red-700" />
                  <Stat label="Skipped" value={report.skipped} cls="text-gray-600" />
                </dl>
              ) : (
                <p className="text-sm text-gray-500">{terminal ? 'No test report for this job.' : 'The report appears after the test run.'}</p>
              )}
            </div>
          </section>

          {pid !== undefined && code && (
            <section aria-labelledby="job-others" className="card">
              <div className="card-header">
                <h2 id="job-others" className="font-medium text-gray-900">
                  Other runs of this TC
                </h2>
              </div>
              <div className="card-body">
                <JobHistory pid={pid} code={code} excludeId={id} showSummary={false} emptyText="No other runs." reloadKey={terminal ? 1 : 0} />
              </div>
            </section>
          )}

          <section aria-labelledby="job-spec" className="card">
            <div className="card-header">
              <h2 id="job-spec" className="font-medium text-gray-900">
                Generated spec
              </h2>
            </div>
            <div className="card-body space-y-2 min-w-0">
              {spec?.path && <p className="font-mono text-xs text-gray-700 break-all">{spec.path}</p>}
              {spec?.content ? (
                <pre tabIndex={0} aria-label="Generated spec source" className="max-h-96 overflow-auto rounded bg-gray-900 text-gray-100 text-xs p-3 whitespace-pre">
                  {spec.content}
                </pre>
              ) : (
                <p className="text-sm text-gray-500">{spec ? 'The file was written; its source is not available here.' : 'No spec generated yet.'}</p>
              )}
            </div>
          </section>
        </div>
      </div>

      {meta.job_type === 'case_create' && <JobConversationCard jobId={id} terminal={terminal} />}
    </div>
  );
}

function Stat({ label, value, cls }: { label: string; value?: number; cls: string }) {
  return (
    <div className="rounded-lg bg-gray-50 px-2 py-2">
      <dt className="text-xs text-gray-500">{label}</dt>
      <dd className={`text-xl font-semibold tabular-nums ${cls}`}>{value ?? '–'}</dd>
    </div>
  );
}

function Timeline({ events, loaded, terminal }: { events: JobEvent[]; loaded: boolean; terminal: boolean }) {
  const box = useRef<HTMLDivElement>(null);
  const [pinned, setPinned] = useState(true);
  const pinnedRef = useRef(true);

  const onScroll = () => {
    const el = box.current;
    if (!el) return;
    const atEnd = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
    pinnedRef.current = atEnd;
    setPinned(atEnd);
  };
  const jump = useCallback(() => {
    const el = box.current;
    if (el) el.scrollTop = el.scrollHeight;
    pinnedRef.current = true;
    setPinned(true);
  }, []);

  // Follow new events only while the reader is at the bottom (scrolling up pauses it).
  useEffect(() => {
    const el = box.current;
    if (el && pinnedRef.current) el.scrollTop = el.scrollHeight;
  }, [events.length]);

  return (
    <div className="relative">
      <div ref={box} onScroll={onScroll} tabIndex={0} role="log" aria-label="Job events" className="max-h-[32rem] min-h-40 overflow-auto font-mono text-xs divide-y divide-gray-100">
        {!loaded && <p className="px-4 py-6 text-sm text-gray-500 font-sans">Loading events…</p>}
        {loaded && events.length === 0 && <p className="px-4 py-6 text-sm text-gray-500 font-sans">{terminal ? 'No events were recorded.' : 'Waiting for the first event…'}</p>}
        {events.map((e) => (
          <div key={e.seq} className={`flex items-start gap-2 px-3 py-1.5 ${LEVEL_CLASS[(e.level ?? 'info').toLowerCase()] ?? ''}`}>
            <time className="shrink-0 text-gray-400 tabular-nums" dateTime={e.at}>
              {fmtTime(e.at)}
            </time>
            {e.level && ['error', 'warn', 'warning'].includes(e.level.toLowerCase()) && <span className="shrink-0 font-semibold uppercase">{e.level}</span>}
            {e.step && <span className={`shrink-0 rounded px-1.5 py-px ${STEP_CLASS}`}>{e.step}</span>}
            <span className="min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{e.message}</span>
          </div>
        ))}
      </div>
      {!pinned && (
        <button
          type="button"
          onClick={jump}
          className="absolute bottom-3 right-4 btn btn-secondary inline-flex items-center gap-1.5 h-8 px-3 text-xs shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        >
          <ArrowDown className="w-3.5 h-3.5" aria-hidden="true" />
          Auto-scroll paused. Jump to latest
        </button>
      )}
    </div>
  );
}

const OPEN_APPROVAL = ['open', 'pending', 'waiting'];

function ApprovalCard({
  jobId,
  approval,
  terminal,
  onAnswered,
  toast,
}: {
  jobId: string;
  approval: JobApproval;
  terminal: boolean;
  onAnswered: () => void;
  toast: ReturnType<typeof useToast>;
}) {
  const [text, setText] = useState('');
  const [busy, setBusy] = useState<ApprovalDecision | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [sent, setSent] = useState<ApprovalDecision | null>(null);
  const open = OPEN_APPROVAL.includes((approval.status ?? 'open').toLowerCase()) && !terminal && !sent;
  const needsText = text.trim() === '';

  const submit = async (decision: ApprovalDecision) => {
    if (busy) return;
    if (decision === 'changes' && needsText) {
      setErr('Describe the changes you want.');
      return;
    }
    setBusy(decision);
    setErr(null);
    try {
      await answerApproval(jobId, decision, text.trim() || undefined);
      setSent(decision);
      toast.success(decision === 'approve' ? 'Approved. The branch will be pushed.' : decision === 'reject' ? 'Rejected.' : 'Changes requested.');
      onAnswered();
    } catch (e) {
      const m = errorText(e);
      setErr(m);
      toast.error(`Could not send your answer: ${m}`);
    } finally {
      setBusy(null);
    }
  };

  return (
    <section aria-labelledby="job-approval" className="card border-l-4 border-l-amber-400">
      <div className="card-header flex flex-wrap items-center gap-2">
        <h2 id="job-approval" className="font-medium text-gray-900">
          Approval
        </h2>
        <span className="badge bg-gray-100 text-gray-700">{sent ? `answered: ${sent}` : (approval.status ?? 'open')}</span>
      </div>
      <div className="card-body space-y-3">
        {approval.gaos_created && <p className="text-sm text-gray-600">Also asked in GAOS Inbox. Whichever answer arrives first is used.</p>}
        {!open && !sent && approval.status && !OPEN_APPROVAL.includes(approval.status.toLowerCase()) && (approval.source || approval.answered_by) && (
          <p className="text-sm text-gray-700" data-testid="answered-via">
            Answered via {answerSourceLabel(approval.source, approval.answered_by) || approval.source}
            {approval.actor && (
              <>
                {' '}
                by <span className="break-words">{approval.actor}</span>
              </>
            )}
            .
          </p>
        )}
        {approval.answer_text && <p className="text-sm text-gray-700 whitespace-pre-wrap break-words">Answer: {approval.answer_text}</p>}
        {open ? (
          <>
            <div>
              <label htmlFor="approval-text" className="block text-sm font-medium text-gray-700 mb-1">
                Comment <span className="font-normal text-gray-500">(required for Needs changes)</span>
              </label>
              <textarea
                id="approval-text"
                value={text}
                onChange={(e) => {
                  setText(e.target.value);
                  setErr(null);
                }}
                rows={3}
                maxLength={1000}
                aria-invalid={!!err && needsText}
                aria-describedby={err ? 'approval-err' : undefined}
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
              />
            </div>
            {err && (
              <p id="approval-err" role="alert" className="text-sm text-red-700 break-words">
                {err}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <SubmitButton loading={busy === 'approve'} loadingLabel="Approving…" disabled={busy !== null} onClick={() => submit('approve')} className="btn btn-primary h-9 px-3 text-sm gap-1.5">
                <Check className="w-4 h-4" aria-hidden="true" /> Approve
              </SubmitButton>
              <SubmitButton loading={busy === 'changes'} loadingLabel="Sending…" disabled={busy !== null} onClick={() => submit('changes')} className="btn btn-secondary h-9 px-3 text-sm gap-1.5">
                <MessageSquareWarning className="w-4 h-4" aria-hidden="true" /> Needs changes
              </SubmitButton>
              <SubmitButton loading={busy === 'reject'} loadingLabel="Rejecting…" disabled={busy !== null} onClick={() => submit('reject')} className="btn btn-danger h-9 px-3 text-sm gap-1.5">
                <X className="w-4 h-4" aria-hidden="true" /> Reject
              </SubmitButton>
            </div>
          </>
        ) : (
          !approval.answer_text && <p className="text-sm text-gray-600">{sent ? 'Your answer was sent.' : 'This approval is closed.'}</p>
        )}
      </div>
    </section>
  );
}
