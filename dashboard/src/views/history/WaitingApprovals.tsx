// "Waiting for approval" on the run history (docs/designs/HISTORY_ANSWER.md): every open approval
// of any job that uses the gate (builds and policy proposals), each with an Answer button.
import { Link } from 'react-router-dom';
import { Hourglass } from 'lucide-react';
import { entityHref, formatMs } from '../knowledge/api';
import { jobHref } from '../knowledge/CaseAutomation';
import { WaitingApproval, productLabel } from './api';
import { JOB_TYPE_LABEL } from './AnswerDialog';

export default function WaitingApprovals({
  items,
  canAnswer,
  onAnswer,
}: {
  items: WaitingApproval[];
  canAnswer: boolean;
  onAnswer: (item: WaitingApproval) => void;
}) {
  if (items.length === 0) return null;
  return (
    <section aria-labelledby="waiting-approvals" className="card border-l-4 border-l-amber-400" data-testid="waiting-approvals">
      <div className="card-header flex items-center gap-2">
        <Hourglass className="w-4 h-4 text-amber-700" aria-hidden="true" />
        <h2 id="waiting-approvals" className="font-medium text-gray-900">
          Waiting for approval ({items.length})
        </h2>
      </div>
      <ul className="divide-y divide-gray-100">
        {items.map((it) => (
          <li key={it.external_id} className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-2 text-sm">
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-x-2">
                <span className="text-xs uppercase tracking-wide text-gray-500">{JOB_TYPE_LABEL[it.job_type] ?? it.job_type}</span>
                {it.case_code ? (
                  <Link to={entityHref(it.product_id, 'test_case', it.case_code)} className="font-mono text-primary-700 hover:underline">
                    {it.case_code}
                  </Link>
                ) : (
                  <span className="text-gray-900 break-words">{it.title || '–'}</span>
                )}
                <span className="text-gray-500">· {productLabel(it.product_name, it.product_id)}</span>
                {it.gaos_created && <span className="badge bg-gray-100 text-gray-700">also in GAOS</span>}
              </div>
              <div className="text-xs text-gray-500 break-all">
                <Link to={jobHref(it.job_id)} className="font-mono text-primary-700 hover:underline">
                  {it.job_id}
                </Link>
                {it.spec_path && <> · {it.spec_path}</>}
                {it.report && (
                  <>
                    {' '}
                    · last run {it.report.passed} passed, {it.report.failed} failed, {it.report.skipped} skipped
                  </>
                )}{' '}
                · waiting {formatMs(it.waiting_ms) || 'just now'}
                {it.round > 1 && <> · round {it.round}</>}
              </div>
            </div>
            {canAnswer && (
              <button type="button" onClick={() => onAnswer(it)} className="btn btn-primary h-8 px-3 text-sm" aria-label={`Answer the approval of ${it.case_code || it.title || it.job_id}`}>
                Answer
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
