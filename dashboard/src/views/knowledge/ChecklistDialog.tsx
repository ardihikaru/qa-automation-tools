import { FormEvent, useEffect, useRef, useState } from 'react';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import { ChecklistCache, ChecklistFormat, ChecklistScope, downloadChecklist, errorText, isAbort } from './api';

interface Props {
  pid: number;
  onClose: () => void;
}

const FORMATS: { id: ChecklistFormat; label: string }[] = [
  { id: 'pdf', label: 'PDF' },
  { id: 'xlsx', label: 'Excel' },
  { id: 'csv', label: 'CSV' },
];

const SCOPES: { id: ChecklistScope; label: string; help: string }[] = [
  { id: 'all', label: 'All features', help: 'Every feature of the catalogue, whatever its status.' },
  { id: 'prod', label: 'Production only', help: 'Only PROD features; optionally only those in production since a period.' },
];

type ProdSelection = 'all' | 'period' | 'unknown';

const PROD_SELECTIONS: { id: ProdSelection; label: string; help: string }[] = [
  { id: 'all', label: 'All PROD features', help: 'Every feature in production; undated ones show \u201c?\u201d in Prod sejak.' },
  { id: 'period', label: 'Released in a period', help: 'Only features whose Prod sejak overlaps the months below. Features without a date are left out.' },
  { id: 'unknown', label: 'Release date unknown', help: 'Only features in production whose Prod sejak is not known yet (\u201c?\u201d), e.g. to fill in the dates.' },
];

const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;

const CACHE_NOTE: Record<ChecklistCache, string> = {
  hit: 'Served from cache',
  miss: 'Generated now',
  regenerated: 'Generated now',
  bypass: 'Generated now; the cache is unavailable',
};

export default function ChecklistDialog({ pid, onClose }: Props) {
  const [format, setFormat] = useState<ChecklistFormat>('pdf');
  const [scope, setScope] = useState<ChecklistScope>('all');
  const [sinceFrom, setSinceFrom] = useState('');
  const [sinceTo, setSinceTo] = useState('');
  const [prodSel, setProdSel] = useState<ProdSelection>('all');
  const [checkColumn, setCheckColumn] = useState(true);
  const [devPeriod, setDevPeriod] = useState(false);
  const [lastUpdate, setLastUpdate] = useState(true);
  const [force, setForce] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const inflight = useRef(false);
  const abort = useRef<AbortController | null>(null);
  const formRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    formRef.current?.querySelector<HTMLElement>('input:checked, input:not([disabled])')?.focus();
    return () => abort.current?.abort();
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (inflight.current) return;
    const rangeError = scope === 'prod' && prodSel === 'period' ? checkRange(sinceFrom, sinceTo) : null;
    if (rangeError) {
      setError(rangeError);
      setNote(null);
      return;
    }
    inflight.current = true;
    setBusy(true);
    setError(null);
    setNote(null);
    const ac = new AbortController();
    abort.current = ac;
    try {
      const prod = scope === 'prod';
      const period = prod && prodSel === 'period';
      const cache = await downloadChecklist(
        pid,
        {
          format,
          scope,
          sinceFrom: period ? sinceFrom : '',
          sinceTo: period ? sinceTo : '',
          prodSince: prod && prodSel === 'unknown' ? 'unknown' : undefined,
          checkColumn: prod ? checkColumn : true,
          devPeriod,
          lastUpdate,
          force,
        },
        ac.signal,
      );
      setNote(cache ? CACHE_NOTE[cache] : 'Downloaded');
    } catch (err) {
      if (!isAbort(err)) setError(errorText(err, 'Could not download the checklist'));
    } finally {
      inflight.current = false;
      abort.current = null;
      setBusy(false);
    }
  }

  return (
    <Modal title="Download module checklist" onClose={() => !busy && onClose()}>
      <form ref={formRef} onSubmit={submit} className="space-y-4">
        <fieldset disabled={busy} className="space-y-2">
          <legend className="block text-sm font-medium text-gray-700 mb-1">Format</legend>
          <div className="flex flex-wrap gap-x-5 gap-y-2">
            {FORMATS.map((f) => (
              <label key={f.id} className="inline-flex items-center gap-2 text-sm text-gray-800">
                <input type="radio" name="checklist-format" value={f.id} checked={format === f.id} onChange={() => setFormat(f.id)} />
                {f.label}
              </label>
            ))}
          </div>
        </fieldset>

        <fieldset disabled={busy} className="space-y-2">
          <legend className="block text-sm font-medium text-gray-700 mb-1">Scope</legend>
          {SCOPES.map((sc) => (
            <div key={sc.id}>
              <label className="inline-flex items-center gap-2 text-sm text-gray-800">
                <input type="radio" name="checklist-scope" value={sc.id} checked={scope === sc.id} onChange={() => {
                    setScope(sc.id);
                    // The recorded-status date is not a go-live date; off by default for a PROD checklist.
                    setLastUpdate(sc.id === 'all');
                  }} aria-describedby={`checklist-scope-${sc.id}-help`} />
                {sc.label}
              </label>
              <p id={`checklist-scope-${sc.id}-help`} className="text-xs text-gray-500 ml-6">
                {sc.help}
              </p>
            </div>
          ))}
          {scope === 'prod' && (
            <div className="ml-6 space-y-3">
              <fieldset className="space-y-1.5">
                <legend className="block text-xs font-medium text-gray-700 mb-1">Which PROD features</legend>
                {PROD_SELECTIONS.map((o) => (
                  <div key={o.id}>
                    <label className="inline-flex items-center gap-2 text-sm text-gray-800">
                      <input type="radio" name="checklist-prod-selection" value={o.id} checked={prodSel === o.id} onChange={() => setProdSel(o.id)} aria-describedby={`checklist-prod-${o.id}-help`} />
                      {o.label}
                    </label>
                    <p id={`checklist-prod-${o.id}-help`} className="text-xs text-gray-500 ml-6">
                      {o.help}
                    </p>
                    {o.id === 'period' && prodSel === 'period' && (
                      <div className="ml-6 mt-1 flex flex-wrap items-end gap-3">
                        <label className="text-sm text-gray-800">
                          <span className="block text-xs text-gray-600 mb-1">From</span>
                          <input type="month" className="border border-gray-300 rounded-md px-2 py-1 text-sm" value={sinceFrom} onChange={(e) => setSinceFrom(e.target.value)} />
                        </label>
                        <label className="text-sm text-gray-800">
                          <span className="block text-xs text-gray-600 mb-1">To</span>
                          <input type="month" className="border border-gray-300 rounded-md px-2 py-1 text-sm" value={sinceTo} onChange={(e) => setSinceTo(e.target.value)} />
                        </label>
                        {(sinceFrom || sinceTo) && (
                          <button type="button" className="text-xs text-teal-700 underline" onClick={() => { setSinceFrom(''); setSinceTo(''); }}>
                            Clear period
                          </button>
                        )}
                      </div>
                    )}
                  </div>
                ))}
              </fieldset>
              <div>
                <label className="inline-flex items-center gap-2 text-sm text-gray-800">
                  <input type="checkbox" checked={checkColumn} onChange={(e) => setCheckColumn(e.target.checked)} aria-describedby="checklist-cek-help" />
                  Include the Cek column
                </label>
                <p id="checklist-cek-help" className="text-xs text-gray-500 ml-6">
                  Empty tick boxes for checking by hand. Untick when the checklist is only for reading.
                </p>
              </div>
            </div>
          )}
        </fieldset>

        <div>
          <label className="inline-flex items-center gap-2 text-sm text-gray-800">
            <input type="checkbox" checked={devPeriod} disabled={busy} onChange={(e) => setDevPeriod(e.target.checked)} aria-describedby="checklist-dev-period-help" />
            Add the Periode pelaksanaan column
          </label>
          <p id="checklist-dev-period-help" className="text-xs text-gray-500 mt-1 ml-6">
            When each feature was developed (not all features have it yet). The PDF also shows each module&rsquo;s period.
          </p>
        </div>

        <div className="space-y-3">
          <div>
            <label className="inline-flex items-center gap-2 text-sm text-gray-800">
              <input type="checkbox" checked={lastUpdate} disabled={busy} onChange={(e) => setLastUpdate(e.target.checked)} aria-describedby="checklist-last-update-help" />
              Add the date each status was recorded
            </label>
            <p id="checklist-last-update-help" className="text-xs text-gray-500 mt-1 ml-6">
              When this tool recorded the current status (&ldquo;tercatat&rdquo;), not when the feature went live: that is the Prod sejak column. PROD features with a known Prod sejak don&rsquo;t show it in the PDF. Dates marked ~ are approximate (first tracking of this product).
            </p>
          </div>
          <label className="inline-flex items-center gap-2 text-sm text-gray-800">
            <input type="checkbox" checked={force} disabled={busy} onChange={(e) => setForce(e.target.checked)} />
            Force regenerate (ignore cached file)
          </label>
        </div>

        {error && (
          <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2 break-words">
            {error}
          </div>
        )}
        {note && !error && (
          <p role="status" className="text-sm text-green-800 bg-green-50 border border-green-100 rounded-lg px-3 py-2">
            {note}
          </p>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn btn-secondary" disabled={busy} onClick={onClose}>
            {note ? 'Close' : 'Cancel'}
          </button>
          <SubmitButton type="submit" className="btn btn-primary" loading={busy} loadingLabel="Preparing…">
            Download
          </SubmitButton>
        </div>
      </form>
    </Modal>
  );
}

/** Returns an error message for an invalid "Prod since" period, or null when it is valid (both ends optional). */
function checkRange(from: string, to: string): string | null {
  if ((from && !MONTH.test(from)) || (to && !MONTH.test(to))) return 'Months must be YYYY-MM.';
  if (from && to && from > to) return 'The "from" month must not be after the "to" month.';
  return null;
}
