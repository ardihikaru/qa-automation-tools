import { FormEvent, useEffect, useRef, useState } from 'react';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import { ChecklistCache, ChecklistFormat, downloadChecklist, errorText, isAbort } from './api';

interface Props {
  pid: number;
  onClose: () => void;
}

const FORMATS: { id: ChecklistFormat; label: string }[] = [
  { id: 'pdf', label: 'PDF' },
  { id: 'xlsx', label: 'Excel' },
  { id: 'csv', label: 'CSV' },
];

const CACHE_NOTE: Record<ChecklistCache, string> = {
  hit: 'Served from cache',
  miss: 'Generated now',
  regenerated: 'Generated now',
  bypass: 'Generated now; the cache is unavailable',
};

export default function ChecklistDialog({ pid, onClose }: Props) {
  const [format, setFormat] = useState<ChecklistFormat>('pdf');
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
    inflight.current = true;
    setBusy(true);
    setError(null);
    setNote(null);
    const ac = new AbortController();
    abort.current = ac;
    try {
      const cache = await downloadChecklist(pid, { format, lastUpdate, force }, ac.signal);
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

        <div className="space-y-3">
          <div>
            <label className="inline-flex items-center gap-2 text-sm text-gray-800">
              <input type="checkbox" checked={lastUpdate} disabled={busy} onChange={(e) => setLastUpdate(e.target.checked)} aria-describedby="checklist-last-update-help" />
              Add last update to Status column
            </label>
            <p id="checklist-last-update-help" className="text-xs text-gray-500 mt-1 ml-6">
              Dates marked ~ are approximate: they come from the first tracking of this product, not from the exact change.
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
