import { useEffect, useRef, useState } from 'react';
import SubmitButton from '../../components/SubmitButton';
import { ApiError, GAOS_ID_RE, GaosPushResult, Product, api } from './api';

interface Props {
  product: Product;
  /** `gaos_configured` of the products list: the server has a valid GAOS_API_URL and GAOS_API_TOKEN. */
  gaosConfigured: boolean;
  /** Admin and DevOps only; the server enforces it too. */
  canToggle: boolean;
  onChange: (p: Product) => void;
}

const STATUS_STYLE: Record<string, string> = {
  ok: 'bg-green-100 text-green-800',
  partial: 'bg-amber-100 text-amber-800',
  failed: 'bg-red-100 text-red-700',
};

/** Label and class of a last-status chip; text is never colour only. */
export function GaosStatusChip({ status }: { status: string }) {
  if (!status) return <span className="badge bg-gray-100 text-gray-600">not synced yet</span>;
  return <span className={`badge ${STATUS_STYLE[status] ?? 'bg-gray-100 text-gray-600'}`}>{status}</span>;
}

export default function GaosCard({ product, gaosConfigured, canToggle, onChange }: Props) {
  const g = product.gaos;
  const [idDraft, setIdDraft] = useState(g.product_id);
  const [busy, setBusy] = useState<'toggle' | 'save' | 'sync' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const [watching, setWatching] = useState<string | null>(null); // last_sync_at when a push was started
  const inflight = useRef(false);

  useEffect(() => setIdDraft(g.product_id), [g.product_id]);

  // After a push was started in the background, re-read the product until its outcome changes.
  useEffect(() => {
    if (watching === null) return;
    let stopped = false;
    let tries = 0;
    const t = window.setInterval(async () => {
      tries += 1;
      try {
        const p = await api<Product>(`/api/products/${product.id}`);
        if (stopped) return;
        if ((p.gaos.last_sync_at ?? '') !== watching || !p.gaos.integrated) {
          setWatching(null);
          onChange(p);
        }
      } catch {
        /* keep trying */
      }
      if (tries >= 60) setWatching(null);
    }, 3000);
    return () => {
      stopped = true;
      window.clearInterval(t);
    };
  }, [watching, product.id, onChange]);

  const idTrim = idDraft.trim();
  const idValid = GAOS_ID_RE.test(idTrim) && idTrim.length <= 40;
  const idDirty = idTrim !== g.product_id;
  const toggleBlockedWhy = !canToggle
    ? 'Only Admin and DevOps can change this.'
    : !gaosConfigured && !g.integrated
      ? 'Not configured on this server: set GAOS_API_URL and GAOS_API_TOKEN in the backend environment.'
      : null;
  const inputBlockedWhy = !canToggle
    ? 'Only Admin and DevOps can change this.'
    : !gaosConfigured
      ? 'Not configured on this server: set GAOS_API_URL and GAOS_API_TOKEN in the backend environment.'
      : null;

  async function run(kind: 'toggle' | 'save' | 'sync', fn: () => Promise<void>) {
    if (inflight.current) return;
    inflight.current = true;
    setBusy(kind);
    setError(null);
    setInfo(null);
    try {
      await fn();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : e instanceof Error ? e.message : 'Action failed');
    } finally {
      inflight.current = false;
      setBusy(null);
    }
  }

  const put = (body: Record<string, unknown>) =>
    run(body.gaos_integrated === undefined ? 'save' : 'toggle', async () => {
      const p = await api<Product>(`/api/products/${product.id}`, { method: 'PUT', body: JSON.stringify(body) });
      onChange(p);
      if (p.gaos.integrated && (!g.integrated || p.gaos.product_id !== g.product_id)) {
        setInfo('Pushing the test cases to GAOS in the background…');
        setWatching(p.gaos.last_sync_at ?? '');
      }
    });

  const toggle = () => {
    if (g.integrated) return void put({ gaos_integrated: false });
    if (!idValid) return setError('Enter the GAOS product id first (for example p-6).');
    void put({ gaos_integrated: true, gaos_product_id: idTrim });
  };

  const syncNow = () =>
    run('sync', async () => {
      const r = await api<GaosPushResult>(`/api/products/${product.id}/gaos/sync`, { method: 'POST' });
      if (r.status === 'running') {
        setInfo('Still pushing; the result will show here.');
        setWatching(g.last_sync_at ?? '');
      }
      onChange(await api<Product>(`/api/products/${product.id}`));
    });

  const label = 'gaos-switch-label';
  return (
    <div className="card p-4 space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-base font-semibold" id={label}>GAOS integration</h3>
          <p className="text-xs text-gray-500 mt-1">
            Integrated with the GAOS platform: this product's test cases are created on its GAOS kanban after every knowledge sync, when this is
            turned on, and on demand. Planning fields already in GAOS are never changed from here.
          </p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={g.integrated}
          aria-labelledby={label}
          aria-busy={busy === 'toggle'}
          disabled={toggleBlockedWhy !== null || busy !== null}
          title={toggleBlockedWhy ?? undefined}
          onClick={toggle}
          className={`relative shrink-0 w-11 h-6 rounded-full transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed ${
            g.integrated ? 'bg-primary-600' : 'bg-gray-300'
          }`}
        >
          <span
            className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform duration-150 ease-out-strong ${
              g.integrated ? 'translate-x-5' : 'translate-x-0'
            }`}
          />
        </button>
      </div>

      {toggleBlockedWhy && (
        <p role="note" className="text-xs text-amber-800 bg-amber-50 border border-amber-100 rounded px-2 py-1">{toggleBlockedWhy}</p>
      )}

      <div className="flex flex-wrap items-end gap-2">
        <label className="text-sm min-w-0">
          <span className="block text-xs text-gray-600 mb-1">GAOS product id</span>
          <input
            className="w-40 px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent disabled:opacity-50"
            value={idDraft}
            onChange={(e) => setIdDraft(e.target.value)}
            placeholder="p-6"
            maxLength={40}
            spellCheck={false}
            disabled={inputBlockedWhy !== null || busy !== null}
            title={inputBlockedWhy ?? undefined}
            aria-invalid={idTrim !== '' && !idValid}
          />
        </label>
        {idDirty && (
          <SubmitButton
            type="button"
            className="btn btn-secondary text-sm"
            loading={busy === 'save'}
            loadingLabel="Saving…"
            disabled={inputBlockedWhy !== null || busy !== null || (idTrim !== '' && !idValid)}
            onClick={() => void put({ gaos_product_id: idTrim })}
          >
            Save id
          </SubmitButton>
        )}
        <SubmitButton
          type="button"
          className="btn btn-primary text-sm"
          loading={busy === 'sync'}
          loadingLabel="Syncing…"
          disabled={!g.integrated || !gaosConfigured || !canToggle || busy !== null || watching !== null}
          title={!g.integrated ? 'Turn the integration on first' : (inputBlockedWhy ?? undefined)}
          onClick={syncNow}
        >
          Sync to GAOS now
        </SubmitButton>
      </div>
      {idTrim !== '' && !idValid && <p className="text-xs text-red-700">The id looks like p-6 (p, a dash, digits).</p>}

      <div className="text-sm text-gray-700 flex flex-wrap items-center gap-x-3 gap-y-1">
        <span>Last sync:</span>
        <GaosStatusChip status={g.last_status} />
        {g.last_sync_at && <span className="text-xs text-gray-500">{new Date(g.last_sync_at).toLocaleString()}</span>}
      </div>
      {g.last_note && <p className="text-xs text-gray-600 break-words">{g.last_note}</p>}
      {info && <p role="status" className="text-xs text-gray-600">{info}</p>}
      {error && (
        <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">{error}</div>
      )}
    </div>
  );
}
