import { useCallback, useEffect, useRef, useState } from 'react';
import { Pagination } from '../knowledge/ui';
import { ApiError, GaosDecisionFields, GaosDecisionItem, GaosDecisionsPage, api } from './api';

const PAGE_SIZE = 10;

const DECISION_STYLE: Record<string, string> = {
  planned: 'bg-green-100 text-green-800',
  blocked: 'bg-red-100 text-red-700',
  'manual-only': 'bg-amber-100 text-amber-800',
  wont: 'bg-gray-100 text-gray-600',
};

function DecisionChip({ decision }: { decision: string }) {
  return <span className={`badge ${DECISION_STYLE[decision] ?? 'bg-gray-100 text-gray-600'}`}>{decision}</span>;
}

function show(f: GaosDecisionFields | null, field: string): string {
  if (!f) return '—';
  const v = (f as unknown as Record<string, unknown>)[field];
  if (Array.isArray(v)) return v.length ? v.join(', ') : '—';
  return v === undefined || v === null || v === '' ? '—' : String(v);
}

function Differences({ item }: { item: GaosDecisionItem }) {
  if (!item.ours) return <span className="text-xs text-gray-400">not in the QA push</span>;
  if (!item.differs?.length) return <span className="text-xs text-gray-400">—</span>;
  return (
    <ul className="space-y-0.5">
      {item.differs.map((f) => (
        <li key={f} className="text-xs break-words">
          <span className="font-medium">{f}</span>: {show(item.theirs, f)} <span aria-label="becomes">→</span> {show(item.ours, f)}
        </li>
      ))}
    </ul>
  );
}

export default function GaosDecisions({ productId }: { productId: number }) {
  const [open, setOpen] = useState(false);
  const [onlyDiff, setOnlyDiff] = useState<boolean | null>(null); // null: not decided yet
  const [page, setPage] = useState(1);
  const [data, setData] = useState<GaosDecisionsPage | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const seq = useRef(0);

  const diff = onlyDiff ?? false;

  const load = useCallback(
    async (d: boolean, pg: number) => {
      const my = ++seq.current;
      setLoading(true);
      setError(null);
      try {
        const r = await api<GaosDecisionsPage>(
          `/api/products/${productId}/gaos/decisions?diff=${d ? 1 : 0}&page=${pg}&page_size=${PAGE_SIZE}`,
        );
        if (my !== seq.current) return;
        setData(r);
        // First load: default the filter on when anything differs.
        if (onlyDiff === null && r.differing > 0) {
          setOnlyDiff(true);
        } else if (onlyDiff === null) {
          setOnlyDiff(false);
        }
      } catch (e) {
        if (my !== seq.current) return;
        setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : 'Could not load the decisions');
      } finally {
        if (my === seq.current) setLoading(false);
      }
    },
    [productId, onlyDiff],
  );

  // Fetch when opened and whenever the filter or page changes. The first (unfiltered) load decides
  // the default of the checkbox; a second load then applies it.
  useEffect(() => {
    if (open) void load(diff, page);
  }, [open, diff, page, load]);

  const header = data ? `${data.decisions} decisions from GAOS, ${data.differing} differ from the QA tools' derivation` : null;
  const panelId = `gaos-decisions-${productId}`;

  return (
    <div className="border-t border-gray-100 pt-3">
      <button
        type="button"
        className="text-sm text-primary-700 hover:underline"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((o) => !o)}
      >
        {open ? 'Hide GAOS decisions' : 'Show GAOS decisions'}
      </button>
      {open && (
        <div id={panelId} className="mt-3 space-y-3" aria-busy={loading}>
          {header && <p className="text-sm text-gray-700">{header}</p>}
          <label className="inline-flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={diff}
              onChange={(e) => {
                setOnlyDiff(e.target.checked);
                setPage(1);
              }}
            />
            Only cases that differ
          </label>

          {error && (
            <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2 flex items-center justify-between gap-3">
              <span className="min-w-0 break-words">{error}</span>
              <button type="button" className="btn btn-secondary text-sm shrink-0" disabled={loading} onClick={() => void load(diff, page)}>
                Retry
              </button>
            </div>
          )}

          {!error && loading && !data && (
            <div className="space-y-2" role="status" aria-label="Loading GAOS decisions">
              {[0, 1, 2].map((i) => (
                <div key={i} className="h-8 rounded bg-gray-100 animate-pulse" />
              ))}
            </div>
          )}

          {!error && data && data.items.length === 0 && !loading && (
            <p className="text-sm text-gray-500">
              {data.decisions === 0 ? 'GAOS has not pushed any decisions yet.' : 'No cases differ.'}
            </p>
          )}

          {data && data.items.length > 0 && (
            <div className={`overflow-x-auto ${loading ? 'opacity-60' : ''}`}>
              <table className="min-w-full text-sm text-left">
                <thead>
                  <tr className="text-xs text-gray-500 border-b border-gray-200">
                    <th scope="col" className="py-2 pr-3 font-medium">Case code</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Decision</th>
                    <th scope="col" className="py-2 pr-3 font-medium">Reason</th>
                    <th scope="col" className="py-2 font-medium">Differences</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((it) => (
                    <tr key={it.code} className="border-b border-gray-100 align-top">
                      <td className="py-2 pr-3 font-mono text-xs whitespace-nowrap">{it.code}</td>
                      <td className="py-2 pr-3"><DecisionChip decision={it.decision} /></td>
                      <td className="py-2 pr-3 text-xs text-gray-700 break-words min-w-[10rem]">{it.reason || '—'}</td>
                      <td className="py-2 min-w-[14rem]"><Differences item={it} /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {data && data.total > PAGE_SIZE && (
            <Pagination page={data.page} pageSize={data.page_size || PAGE_SIZE} total={data.total} onPage={setPage} disabled={loading} noun="decisions" />
          )}
        </div>
      )}
    </div>
  );
}
