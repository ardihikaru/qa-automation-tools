import { useRef, useState } from 'react';
import SubmitButton from '../../components/SubmitButton';
import { ApiError, CaseResultsSummary, Product, api } from './api';

const MAX_BYTES = 24 * 1024 * 1024;
const MAX_REF = 120;
const SHOWN_CODES = 20;

export default function CaseResultsCard({ product }: { product: Product }) {
  const [file, setFile] = useState<File | null>(null);
  const [runRef, setRunRef] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [summary, setSummary] = useState<CaseResultsSummary | null>(null);
  const inflight = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);

  function pick(f: File | null) {
    setSummary(null);
    if (f && f.size > MAX_BYTES) {
      setFile(null);
      if (inputRef.current) inputRef.current.value = '';
      setError(`"${f.name}" is ${(f.size / 1024 / 1024).toFixed(1)} MB; the limit is 24 MB.`);
      return;
    }
    setError(null);
    setFile(f);
  }

  async function upload() {
    if (inflight.current) return;
    if (!file) return setError('Choose a results.json file first.');
    const ref = runRef.trim();
    if (/[^\x20-\x7E]/.test(ref)) return setError('Run reference may only contain printable ASCII characters.');
    inflight.current = true;
    setBusy(true);
    setError(null);
    setSummary(null);
    try {
      const qs = ref ? `?run_ref=${encodeURIComponent(ref)}` : '';
      setSummary(await api<CaseResultsSummary>(`/api/products/${product.id}/case-results${qs}`, { method: 'POST', body: file }));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : 'Upload failed');
    } finally {
      inflight.current = false;
      setBusy(false);
    }
  }

  const codes = summary?.unmatched ?? [];

  return (
    <form
      className="card p-4 space-y-3"
      aria-label="Test results"
      onSubmit={(e) => {
        e.preventDefault();
        void upload();
      }}
    >
      <div>
        <h3 className="text-base font-semibold">Test results</h3>
        <p className="text-xs text-gray-500 mt-1">
          Upload results.json from the automation repo's Playwright run. Test titles must start with a TC-XXX-NNN code.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="block min-w-0">
          <span className="block text-sm font-medium text-gray-700 mb-1">Results file (.json, max 24 MB)</span>
          <input
            ref={inputRef}
            type="file"
            accept=".json,application/json"
            disabled={busy}
            onChange={(e) => pick(e.target.files?.[0] ?? null)}
            className="block w-full text-sm text-gray-700 file:mr-3 file:px-3 file:py-2 file:rounded-lg file:border-0 file:bg-gray-100 file:text-gray-700"
          />
        </label>
        <label className="block min-w-0">
          <span className="block text-sm font-medium text-gray-700 mb-1">Run reference (optional)</span>
          <input
            value={runRef}
            maxLength={MAX_REF}
            disabled={busy}
            onChange={(e) => setRunRef(e.target.value)}
            placeholder="CI run id"
            className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />
        </label>
      </div>
      {error && (
        <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2 break-words">
          {error}
        </div>
      )}
      {summary && (
        <div role="status" className="text-sm text-green-800 bg-green-50 border border-green-100 rounded-lg px-3 py-2 space-y-1 break-words">
          <p>
            {summary.cases} cases with a result in {summary.tests} tests: {summary.stored} updated, {summary.stale} unchanged or older than the
            stored run
          </p>
          {summary.unmatched_count > 0 && (
            <div className="text-amber-800">
              <p>{summary.unmatched_count} {summary.unmatched_count === 1 ? "code is not an active case" : "codes are not active cases"} of this product</p>
              <p className="font-mono text-xs break-all">
                {codes.slice(0, SHOWN_CODES).join(', ')}
                {summary.unmatched_count > Math.min(codes.length, SHOWN_CODES) ? ', …' : ''}
              </p>
            </div>
          )}
        </div>
      )}
      <SubmitButton type="submit" className="btn btn-primary text-sm" loading={busy} loadingLabel="Uploading…" disabled={!file}>
        Upload
      </SubmitButton>
    </form>
  );
}
