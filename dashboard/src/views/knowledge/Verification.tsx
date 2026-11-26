// Human verification of a test case: verdict icon, the control (page) and the history list.
import { useState } from 'react';
import { Check, CircleSlash, X } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import { useToast } from '../../components/Toast';
import { Verification, VerificationEvent, Verdict, errorText, listVerificationHistory, putVerification } from './api';
import { useLoad } from './hooks';
import { Disclosure, ErrorNote } from './ui';
import { formatInZone, useSystemTimezone } from '../../lib/time';

export function VerdictIcon({ verdict }: { verdict: Verdict | 'cleared' | null | undefined }) {
  if (verdict === 'valid') return <Check className="w-4 h-4 text-green-600 shrink-0" aria-label="valid" />;
  if (verdict === 'invalid') return <X className="w-4 h-4 text-red-600 shrink-0" aria-label="invalid" />;
  if (verdict === 'cleared') return <CircleSlash className="w-4 h-4 text-gray-400 shrink-0" aria-label="cleared" />;
  return (
    <span className="text-gray-400" aria-label="not verified">
      –
    </span>
  );
}

/** Calls PUT verification with an in-flight guard; `busy` is the verdict being sent. */
export function useVerify(pid: number, code: string, onDone: (v: Verification | null) => void) {
  const toast = useToast();
  const [busy, setBusy] = useState<Verdict | 'clear' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const run = async (verdict: Verdict | null, note?: string) => {
    if (busy) return;
    setBusy(verdict ?? 'clear');
    setError(null);
    try {
      const v = await putVerification(pid, code, verdict, note?.trim() || undefined);
      onDone(v);
      toast.success(verdict ? `${code} marked ${verdict}.` : `Verification of ${code} cleared.`);
    } catch (e) {
      const m = errorText(e, 'Could not save the verification');
      setError(m);
      toast.error(m);
    } finally {
      setBusy(null);
    }
  };
  return { busy, error, run };
}

function HistoryRow({ ev, tz }: { ev: VerificationEvent; tz: string }) {
  return (
    <li className="flex items-start gap-2 px-3 py-1.5 text-sm min-w-0">
      <span className="mt-0.5">
        <VerdictIcon verdict={ev.verdict} />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-gray-800 break-words">
          {ev.verdict === 'cleared' ? 'Cleared by ' : ''}
          <span className="font-medium">{ev.verified_by_name || 'Unknown'}</span>
          <time dateTime={ev.verified_at} className="ml-2 text-xs text-gray-500 tabular-nums whitespace-nowrap">
            {formatInZone(ev.verified_at, tz)}
          </time>
        </p>
        {ev.note && <p className="text-xs text-gray-600 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{ev.note}</p>}
      </div>
    </li>
  );
}

export function VerificationHistory({ pid, code, reloadKey }: { pid: number; code: string; reloadKey: number }) {
  const tz = useSystemTimezone();
  const { data, error, reload } = useLoad(`${pid}:${code}:vh:${reloadKey}`, (s) => listVerificationHistory(pid, code, 50, s));
  if (error) return <ErrorNote onRetry={reload}>{error}</ErrorNote>;
  if (!data) return <p className="text-sm text-gray-500">Loading verification history…</p>;
  return (
    <Disclosure key={data.total > 0 ? 'open' : 'closed'} defaultOpen={data.total > 0} summary={<>Verification history ({data.total.toLocaleString()})</>}>
      {data.history.length === 0 ? (
        <p className="text-sm text-gray-500">Never verified.</p>
      ) : (
        <>
          <ul className="rounded-lg border border-gray-200 bg-white divide-y divide-gray-100 overflow-hidden max-h-72 overflow-y-auto">
            {data.history.map((ev) => (
              <HistoryRow key={ev.id} ev={ev} tz={tz} />
            ))}
          </ul>
          {data.total > data.history.length && <p className="mt-1 text-xs text-gray-500">Showing the latest {data.history.length} of {data.total.toLocaleString()}.</p>}
        </>
      )}
    </Disclosure>
  );
}

interface Props {
  pid: number;
  code: string;
  initial: Verification | null | undefined;
  canWrite: boolean;
  removed?: boolean;
}

/** Test case page section body: current state, optional note, actions, history. */
export default function VerificationControl({ pid, code, initial, canWrite, removed }: Props) {
  const tz = useSystemTimezone();
  const [current, setCurrent] = useState<Verification | null>(initial ?? null);
  const [note, setNote] = useState('');
  const [hKey, setHKey] = useState(0);
  const { busy, error, run } = useVerify(pid, code, (v) => {
    setCurrent(v);
    setNote('');
    setHKey((n) => n + 1);
  });

  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-gray-200 bg-white px-4 py-3 text-sm space-y-1">
        {current ? (
          <>
            <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-gray-800">
              <VerdictIcon verdict={current.verdict} />
              <span className="font-medium">{current.verdict === 'valid' ? 'Valid' : 'Invalid'}</span>
              <span className="text-gray-600">
                by {current.verified_by_name}{' '}
                <time dateTime={current.verified_at} className="tabular-nums">
                  {formatInZone(current.verified_at, tz)}
                </time>
              </span>
            </p>
            {current.note && <p className="text-gray-700 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{current.note}</p>}
          </>
        ) : (
          <p className="text-gray-600">Not verified by a person yet.</p>
        )}
      </div>

      {canWrite && !removed && (
        <div className="space-y-2">
          <label className="block text-sm text-gray-700" htmlFor="verification-note">
            Note <span className="text-gray-500">(optional)</span>
          </label>
          <textarea
            id="verification-note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={2}
            maxLength={500}
            className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
          />
          <div className="flex flex-wrap gap-2">
            <SubmitButton loading={busy === 'valid'} loadingLabel="Saving…" disabled={busy !== null} onClick={() => run('valid', note)} className="btn btn-secondary h-8 px-3 text-sm gap-1.5">
              <Check className="w-4 h-4 text-green-600" aria-hidden="true" /> Mark valid
            </SubmitButton>
            <SubmitButton loading={busy === 'invalid'} loadingLabel="Saving…" disabled={busy !== null} onClick={() => run('invalid', note)} className="btn btn-secondary h-8 px-3 text-sm gap-1.5">
              <X className="w-4 h-4 text-red-600" aria-hidden="true" /> Mark invalid
            </SubmitButton>
            {current && (
              <SubmitButton loading={busy === 'clear'} loadingLabel="Clearing…" disabled={busy !== null} onClick={() => run(null, note)} className="btn btn-secondary h-8 px-3 text-sm">
                Clear
              </SubmitButton>
            )}
          </div>
          {error && (
            <p role="alert" className="text-sm text-red-700 break-words">
              {error}
            </p>
          )}
        </div>
      )}

      <VerificationHistory pid={pid} code={code} reloadKey={hKey} />
    </div>
  );
}
