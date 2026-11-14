import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Bot } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import { useToast } from '../../components/Toast';
import { automateEpic, cancelEpicBatch, EpicAutomation as EpicState, EpicCaseState, errorText, getEpicAutomation } from './api';
import { jobHref } from './CaseAutomation';

const STATE_LABEL: Record<EpicCaseState, string> = {
  pending: 'waiting',
  active: 'working',
  awaiting_approval: 'needs your approval',
  pr: 'PR opened',
  declined: 'declined',
  failed: 'failed',
  needs_person: 'needs a person',
  skipped: 'skipped',
};
const STATE_CLASS: Record<EpicCaseState, string> = {
  pending: 'bg-gray-100 text-gray-700',
  active: 'bg-blue-100 text-blue-800',
  awaiting_approval: 'bg-amber-100 text-amber-800',
  pr: 'badge-success',
  declined: 'bg-gray-100 text-gray-700',
  failed: 'badge-error',
  needs_person: 'badge-error',
  skipped: 'bg-gray-100 text-gray-500',
};

interface Props {
  pid: number;
  epic: string;
}

/**
 * "Automate epic": start a batch of an epic's planned cases and follow it. The server runs two
 * cases at a time through the same job as Create automation; a person still approves each spec.
 * Shown only to a user who may verify, for a product with an automation repo.
 */
export default function EpicAutomation({ pid, epic }: Props) {
  const toast = useToast();
  const [state, setState] = useState<EpicState | null>(null);
  const [error, setError] = useState('');
  const [max, setMax] = useState(10);
  const [busy, setBusy] = useState(false);

  const load = useCallback(
    (signal?: AbortSignal) =>
      getEpicAutomation(pid, epic, signal)
        .then((s) => {
          if (signal?.aborted) return;
          setState(s);
          setError('');
        })
        .catch((e) => !signal?.aborted && setError(errorText(e))),
    [pid, epic],
  );

  useEffect(() => {
    setState(null);
    setError('');
    const ac = new AbortController();
    load(ac.signal);
    return () => ac.abort();
  }, [load]);

  const running = state?.batch?.status === 'running';
  useEffect(() => {
    if (!running) return;
    const ac = new AbortController();
    const h = window.setInterval(() => load(ac.signal), 5000);
    return () => {
      ac.abort();
      window.clearInterval(h);
    };
  }, [running, load]);

  if (!state && !error) return <div className="card p-3 text-sm text-gray-500" aria-busy="true">Loading automation for {epic}…</div>;
  if (error && !state)
    return (
      <div className="card p-3 text-sm text-red-700" role="alert">
        {error}{' '}
        <button type="button" className="underline" onClick={() => load()}>
          Retry
        </button>
      </div>
    );
  if (!state || !state.repo) return null;

  const batch = state.batch;
  const start = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const r = await automateEpic(pid, epic, max);
      toast.success(`Batch started: ${r.planned.length} case${r.planned.length === 1 ? '' : 's'} of ${epic}.`);
      await load();
    } catch (e) {
      toast.error(`Could not start the batch: ${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const cancel = async () => {
    if (!batch || busy || !window.confirm(`Cancel the batch for ${epic}? Jobs already running keep going; reject their approvals to stop them.`)) return;
    setBusy(true);
    try {
      await cancelEpicBatch(batch.id);
      toast.success('Batch cancelled.');
      await load();
    } catch (e) {
      toast.error(`Could not cancel: ${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };

  const counts = batch ? batch.cases.reduce<Record<string, number>>((m, c) => ((m[c.state] = (m[c.state] ?? 0) + 1), m), {}) : {};

  return (
    <section className="card p-3 space-y-2" aria-label={`Automate epic ${epic}`}>
      <div className="flex flex-wrap items-center gap-3">
        <Bot className="w-4 h-4 text-gray-500" aria-hidden="true" />
        <h3 className="text-sm font-semibold text-gray-900">Automate epic {epic}</h3>
        {batch && (
          <span className={`badge ${running ? 'bg-blue-100 text-blue-800' : 'bg-gray-100 text-gray-700'}`} data-testid="epic-batch-status">
            {batch.status}
            {batch.note ? ` · ${batch.note}` : ''}
          </span>
        )}
        <span className="flex-1" />
        {running ? (
          <SubmitButton className="btn btn-secondary h-8 px-3 text-sm" loading={busy} loadingLabel="Cancelling…" onClick={cancel}>
            Cancel batch
          </SubmitButton>
        ) : state.eligible > 0 ? (
          <span className="inline-flex items-center gap-2 text-sm">
            <label className="inline-flex items-center gap-1.5 text-gray-700">
              Cases
              <input
                type="number"
                min={1}
                max={25}
                value={max}
                onChange={(e) => setMax(Math.max(1, Math.min(25, Number(e.target.value) || 1)))}
                aria-label="Maximum cases in this batch"
                className="w-16 rounded border border-gray-300 px-2 h-8 text-sm"
              />
            </label>
            <SubmitButton className="btn btn-primary h-8 px-3 text-sm" loading={busy} loadingLabel="Starting…" onClick={start}>
              Start
            </SubmitButton>
          </span>
        ) : null}
      </div>
      {!batch || !running ? (
        <p className="text-sm text-gray-600">
          {state.eligible > 0
            ? `${state.eligible} planned case${state.eligible === 1 ? '' : 's'} without automation. The server works through them two at a time; you approve each spec before any PR opens.`
            : 'Nothing to automate in this epic: no planned case is waiting.'}
        </p>
      ) : null}
      {batch && (
        <>
          <p className="text-xs text-gray-500">
            {Object.entries(counts).map(([k, n]) => `${n} ${STATE_LABEL[k as EpicCaseState] ?? k}`).join(' · ')}
          </p>
          <ul className="divide-y divide-gray-100 text-sm">
            {batch.cases.map((c) => (
              <li key={c.code} className="flex flex-wrap items-center gap-2 py-1">
                <span className="w-6 text-right text-xs text-gray-400">{c.seq}</span>
                <code className="text-xs">{c.code}</code>
                <span className={`badge whitespace-nowrap ${STATE_CLASS[c.state]}`}>{STATE_LABEL[c.state]}</span>
                {c.job_id && (
                  <Link to={jobHref(c.job_id)} className="text-xs text-primary-700 hover:underline">
                    job
                  </Link>
                )}
                {c.detail && <span className="text-xs text-gray-500 truncate max-w-[28rem]" title={c.detail}>{c.detail}</span>}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}
