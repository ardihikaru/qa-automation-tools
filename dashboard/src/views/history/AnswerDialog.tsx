// Answer a waiting approval from the run history (docs/designs/HISTORY_ANSWER.md). The same three
// options as the GAOS Inbox item; when the item exists in GAOS the answer is relayed there.
import { FormEvent, useState } from 'react';
import { Link } from 'react-router-dom';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import { ApiError } from '../products/api';
import { entityHref, errorText, formatMs } from '../knowledge/api';
import { jobHref } from '../knowledge/CaseAutomation';
import { AnswerResult, Decision, WaitingApproval, answerFromHistory, answerSourceLabel } from './api';

const OPTIONS: { value: Decision; label: string; hint: string }[] = [
  { value: 'approve', label: 'Approve', hint: 'Commit, push the branch and open the pull request.' },
  { value: 'reject', label: 'Reject', hint: 'Discard what was generated.' },
  { value: 'changes', label: 'Needs changes', hint: 'Say what to change; it is regenerated.' },
];

const MAX_TEXT = 1000;

export const JOB_TYPE_LABEL: Record<string, string> = { case_create: 'Build', policy_propose: 'Policy proposal' };

/** The message for a 409 of the approval route, by its code. */
function conflictText(body: Record<string, unknown>): string {
  const who = typeof body.actor === 'string' && body.actor ? ` by ${body.actor}` : '';
  const via = answerSourceLabel(typeof body.source === 'string' ? body.source : undefined);
  const status = typeof body.status === 'string' ? body.status : 'answered';
  switch (body.code) {
    case 'answered_in_gaos':
      return `Someone answered first in GAOS${who}: ${status}. Their answer was applied.`;
    case 'answered_here':
      return `It was answered in the QA tools first${via ? ` (${via})` : ''}${who}: ${status}. GAOS shows your answer, which was not used.`;
    case 'newer_round':
      return 'The job has asked again since you opened this. Refresh to see the new round.';
    case 'in_flight':
      return 'An answer to this approval is being sent right now.';
    default:
      return `This approval was already answered${via ? ` via ${via}` : ''}${who}: ${status}.`;
  }
}

export default function AnswerDialog({ item, onClose, onAnswered }: { item: WaitingApproval; onClose: () => void; onAnswered: () => void }) {
  const [decision, setDecision] = useState<Decision | null>(null);
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [done, setDone] = useState<{ ok: boolean; msg: string; note?: string } | null>(null);

  const what = item.job_type === 'policy_propose' ? item.title || 'a learned policy' : item.case_code;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy || done) return;
    if (!decision) return setErr('Choose an answer.');
    if (decision === 'changes' && !text.trim()) return setErr('Say what to change.');
    setBusy(true);
    setErr(null);
    try {
      const r: AnswerResult = await answerFromHistory(item.job_id, item.external_id, decision, text.trim());
      setDone({
        ok: true,
        msg: r.delivered === 'gaos' ? 'Sent to GAOS and applied here. The job continues.' : 'Applied in the QA tools. The job continues.',
        note: r.note,
      });
      onAnswered();
    } catch (e2) {
      if (e2 instanceof ApiError && e2.status === 409) {
        setDone({ ok: false, msg: conflictText(e2.body) });
        onAnswered();
      } else {
        setErr(errorText(e2, 'Could not send your answer'));
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`Answer: ${JOB_TYPE_LABEL[item.job_type] ?? item.job_type} ${what}`} onClose={() => !busy && onClose()}>
      <dl className="grid grid-cols-[auto,1fr] gap-x-3 gap-y-1 text-sm mb-4">
        <dt className="text-gray-500">Product</dt>
        <dd className="min-w-0 break-words">{item.product_name || `Product ${item.product_id}`}</dd>
        {item.case_code && (
          <>
            <dt className="text-gray-500">Case</dt>
            <dd>
              <Link to={entityHref(item.product_id, 'test_case', item.case_code)} className="font-mono text-primary-700 hover:underline">
                {item.case_code}
              </Link>
            </dd>
          </>
        )}
        {item.spec_path && (
          <>
            <dt className="text-gray-500">File</dt>
            <dd className="font-mono text-xs break-all">{item.spec_path}</dd>
          </>
        )}
        {item.report && (
          <>
            <dt className="text-gray-500">Last run</dt>
            <dd>
              {item.report.passed} passed · {item.report.failed} failed · {item.report.skipped} skipped
            </dd>
          </>
        )}
        <dt className="text-gray-500">Waiting</dt>
        <dd>
          {formatMs(item.waiting_ms) || 'just now'}
          {item.round > 1 && <> · round {item.round}</>}
        </dd>
      </dl>
      <p className="text-sm mb-4">
        <Link to={jobHref(item.job_id)} className="text-primary-700 hover:underline">
          Open the job
        </Link>{' '}
        <span className="text-gray-500">to read the generated file and its run log before you decide.</span>
      </p>

      {done ? (
        <div className="space-y-3">
          <p role="status" className={`text-sm ${done.ok ? 'text-emerald-800' : 'text-amber-800'}`}>
            {done.msg}
          </p>
          {done.note && <p className="text-sm text-gray-600 break-words">{done.note}</p>}
          <div className="flex justify-end">
            <button type="button" className="btn btn-secondary h-9 px-3 text-sm" onClick={onClose}>
              Close
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <fieldset>
            <legend className="text-sm font-medium text-gray-700 mb-1">Your answer</legend>
            <div className="space-y-1">
              {OPTIONS.map((o) => (
                <label key={o.value} className="flex items-start gap-2 rounded-lg px-2 py-1.5 hover:bg-gray-50 cursor-pointer">
                  <input
                    type="radio"
                    name="decision"
                    value={o.value}
                    checked={decision === o.value}
                    onChange={() => {
                      setDecision(o.value);
                      setErr(null);
                    }}
                    className="mt-1"
                  />
                  <span className="text-sm">
                    <span className="font-medium text-gray-900">{o.label}</span> <span className="text-gray-500">{o.hint}</span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
          <div>
            <label htmlFor="answer-text" className="block text-sm font-medium text-gray-700 mb-1">
              Comment <span className="font-normal text-gray-500">({decision === 'changes' ? 'required' : 'optional'})</span>
            </label>
            <textarea
              id="answer-text"
              value={text}
              onChange={(e) => {
                setText(e.target.value);
                setErr(null);
              }}
              rows={3}
              maxLength={MAX_TEXT}
              aria-invalid={!!err}
              aria-describedby={err ? 'answer-err' : undefined}
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
            />
          </div>
          <p className="text-xs text-gray-500">
            {item.relay ? 'Your answer is sent to GAOS, which closes its Inbox item, and applied here.' : 'Your answer is applied in the QA tools.'} The first answer wins.
          </p>
          {err && (
            <p id="answer-err" role="alert" className="text-sm text-red-700 break-words">
              {err}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <button type="button" className="btn btn-secondary h-9 px-3 text-sm" onClick={onClose} disabled={busy}>
              Cancel
            </button>
            <SubmitButton type="submit" loading={busy} loadingLabel={item.relay ? 'Sending to GAOS…' : 'Sending…'} disabled={busy} className="btn btn-primary h-9 px-3 text-sm">
              Send answer
            </SubmitButton>
          </div>
        </form>
      )}
    </Modal>
  );
}
