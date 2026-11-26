import { useCallback, useEffect, useState } from 'react';
import { RefreshCw, ShieldCheck, ShieldAlert } from 'lucide-react';
import SubmitButton from '../components/SubmitButton';
import { useAuth } from '../auth/AuthContext';
import { DEFAULT_TIMEZONE, formatInZone, setSystemTimezone } from '../lib/time';

interface SettingsResponse {
  ai: {
    provider: string;
    gateway: string;
    gateway_host: string;
    upstream: string;
    model: string;
    no_fallback: boolean;
    timeout_seconds: number;
    health: string;
    pinned_to_claude_v3: boolean;
  };
  executor: { host: string; status: string };
  database: string;
  timezone?: string;
}

const ZONES = ['Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura', 'Asia/Singapore', 'UTC'];

function statusBadge(status: string): string {
  if (status === 'ok' || status === 'connected' || status === 'configured') return 'badge badge-success';
  if (status === 'not_configured' || status === 'degraded') return 'badge badge-warning';
  return 'badge badge-error';
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-4 py-2 border-b border-gray-100 last:border-0">
      <dt className="sm:w-48 shrink-0 text-sm text-gray-500">{label}</dt>
      <dd className="text-sm text-gray-900 break-all">{children}</dd>
    </div>
  );
}

function TimezoneCard({ zone, canEdit }: { zone: string; canEdit: boolean }) {
  const [value, setValue] = useState(zone);
  const [saved, setSaved] = useState(zone);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    setValue(zone);
    setSaved(zone);
  }, [zone]);
  const options = ZONES.includes(saved) ? ZONES : [saved, ...ZONES];

  const save = async () => {
    if (busy) return;
    setBusy(true);
    setErr(null);
    try {
      const res = await fetch('/api/settings/timezone', {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ timezone: value }),
      });
      if (!res.ok) {
        const b = await res.json().catch(() => ({}));
        throw new Error(b?.error || `Could not save the timezone (${res.status})`);
      }
      setSaved(value);
      setSystemTimezone(value);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Could not save the timezone');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="card">
      <div className="card-header">
        <h3 className="text-lg font-semibold">Timezone</h3>
      </div>
      <div className="card-body space-y-3">
        <p className="text-sm text-gray-600">
          Job times, run history and verification times are shown in this zone. Now: <span className="tabular-nums">{formatInZone(new Date().toISOString(), saved)}</span>
        </p>
        {canEdit ? (
          <div className="flex flex-wrap items-center gap-2">
            <label htmlFor="tz" className="sr-only">
              System timezone
            </label>
            <select id="tz" value={value} onChange={(e) => setValue(e.target.value)} disabled={busy} className="h-9 px-2 border border-gray-300 rounded-lg bg-white text-sm">
              {options.map((z) => (
                <option key={z} value={z}>
                  {z}
                </option>
              ))}
            </select>
            <SubmitButton loading={busy} loadingLabel="Saving…" disabled={value === saved} onClick={save} className="btn btn-primary h-9 px-3 text-sm">
              Save
            </SubmitButton>
          </div>
        ) : (
          <p className="text-sm text-gray-900">{saved}</p>
        )}
        {err && (
          <p role="alert" className="text-sm text-red-700 break-words">
            {err}
          </p>
        )}
      </div>
    </section>
  );
}

export default function SettingsView() {
  const { user } = useAuth();
  const canEditZone = ['Admin', 'DevOps'].includes(user?.role ?? '');
  const [data, setData] = useState<SettingsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch('/api/settings', { credentials: 'include' });
      if (!res.ok) throw new Error(`Failed to load settings (${res.status})`);
      setData(await res.json());
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load settings');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <p className="text-sm text-gray-500">Read-only. Secrets are never shown.</p>
        </div>
        <SubmitButton
          onClick={load}
          loading={loading}
          loadingLabel="Refreshing…"
          className="btn btn-secondary"
        >
          <RefreshCw className="w-4 h-4" /> Refresh
        </SubmitButton>
      </div>

      {error && (
        <div className="mb-4 text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">{error}</div>
      )}

      {data && (
        <div className="grid gap-6 lg:grid-cols-2 mb-6">
          <section className="card">
            <div className="card-header flex items-center justify-between">
              <h3 className="text-lg font-semibold">AI</h3>
              {data.ai.pinned_to_claude_v3 ? (
                <span className="badge badge-success inline-flex gap-1">
                  <ShieldCheck className="w-3.5 h-3.5" /> Pinned to claude-v3
                </span>
              ) : (
                <span className="badge badge-error inline-flex gap-1">
                  <ShieldAlert className="w-3.5 h-3.5" /> Not pinned to claude-v3
                </span>
              )}
            </div>
            <dl className="card-body">
              <Row label="Provider">{data.ai.provider || '—'}</Row>
              <Row label="Gateway">
                {data.ai.gateway} <span className="text-gray-500">({data.ai.gateway_host || 'unset'})</span>
              </Row>
              <Row label="Upstream">{data.ai.upstream || '—'}</Row>
              <Row label="Model">{data.ai.model}</Row>
              <Row label="Fallback">{data.ai.no_fallback ? 'Disabled (no_fallback)' : 'Enabled'}</Row>
              <Row label="Timeout">{data.ai.timeout_seconds}s</Row>
              <Row label="Health">
                <span className={statusBadge(data.ai.health)}>{data.ai.health}</span>
              </Row>
            </dl>
          </section>

          <section className="card">
            <div className="card-header">
              <h3 className="text-lg font-semibold">Services</h3>
            </div>
            <dl className="card-body">
              <Row label="Database">
                <span className={statusBadge(data.database)}>{data.database}</span>
              </Row>
              <Row label="Playwright executor">
                <span className={statusBadge(data.executor.status)}>{data.executor.status}</span>{' '}
                <span className="text-gray-500">{data.executor.host || 'not configured'}</span>
              </Row>
            </dl>
          </section>
        </div>
      )}

      {data && (
        <div className="mb-6">
          <TimezoneCard zone={data.timezone || DEFAULT_TIMEZONE} canEdit={canEditZone} />
        </div>
      )}

      <section className="card">
        <div className="card-header">
          <h3 className="text-lg font-semibold">How it works</h3>
        </div>
        <div className="card-body grid gap-6 xl:grid-cols-3 items-start">
          <div className="xl:col-span-2 min-w-0">
            <ArchitectureDiagram />
          </div>
          <ol className="space-y-2 text-sm text-gray-700 list-decimal list-inside">
            <li>The dashboard calls the Go backend on same-origin <code>/api</code> (cookie auth).</li>
            <li>Run QA / Generate docs: the backend submits a run to <b>playwright-executor</b> and polls it.</li>
            <li>The executor drives Chromium against SUT staging (one run at a time, 2.5 GiB / 60% CPU cap).</li>
            <li>Issues are stored in MySQL; docs are written by <b>claude-v3</b> through <b>ai-manager</b>.</li>
            <li>Every LLM call goes ai-manager → claude-v3, with fallback disabled.</li>
          </ol>
        </div>
      </section>
    </div>
  );
}

function Box({ x, y, w = 130, label, sub, tone = 'blue' }: { x: number; y: number; w?: number; label: string; sub?: string; tone?: 'blue' | 'green' | 'gray' }) {
  const fill = tone === 'green' ? '#dcfce7' : tone === 'gray' ? '#f3f4f6' : '#dbeafe';
  const stroke = tone === 'green' ? '#16a34a' : tone === 'gray' ? '#9ca3af' : '#2563eb';
  return (
    <g>
      <rect x={x} y={y} width={w} height={48} rx={8} fill={fill} stroke={stroke} strokeWidth={1.5} />
      <text x={x + w / 2} y={y + (sub ? 20 : 29)} textAnchor="middle" fontSize={13} fontWeight={600} fill="#111827">{label}</text>
      {sub && <text x={x + w / 2} y={y + 37} textAnchor="middle" fontSize={10.5} fill="#4b5563">{sub}</text>}
    </g>
  );
}

function Arrow({ x1, y1, x2, y2 }: { x1: number; y1: number; x2: number; y2: number }) {
  return <line x1={x1} y1={y1} x2={x2} y2={y2} stroke="#6b7280" strokeWidth={1.5} markerEnd="url(#arrow)" />;
}

function ArchitectureDiagram() {
  return (
    <div className="overflow-x-auto">
      <svg viewBox="0 0 760 270" className="w-full min-w-[640px] max-h-[420px]" role="img" aria-label="System architecture">
        <defs>
          <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0 0L10 5L0 10z" fill="#6b7280" />
          </marker>
        </defs>
        <Box x={10} y={20} label="Dashboard" sub="React SPA" />
        <Box x={200} y={20} label="ai-backend" sub="Go REST (Docker)" />
        <Box x={400} y={20} label="MySQL" sub="qa_automation (db-host)" tone="gray" w={150} />
        <Arrow x1={140} y1={44} x2={200} y2={44} />
        <Arrow x1={330} y1={44} x2={400} y2={44} />

        <Box x={200} y={120} label="ai-manager" sub="db-host :8099" />
        <Box x={400} y={120} label="claude-v3" sub="ai-claude-cli :8105" tone="green" w={150} />
        <Arrow x1={265} y1={68} x2={265} y2={120} />
        <Arrow x1={330} y1={144} x2={400} y2={144} />

        <Box x={10} y={200} w={170} label="playwright-executor" sub="systemd, app-host :8141" />
        <Box x={250} y={200} w={150} label="SUT staging" sub="Chromium target" tone="gray" />
        <Arrow x1={230} y1={68} x2={110} y2={200} />
        <Arrow x1={180} y1={224} x2={250} y2={224} />

        <text x={575} y={50} fontSize={11} fill="#6b7280">Issues, jobs, flows</text>
        <text x={575} y={150} fontSize={11} fill="#16a34a">All LLM calls (pinned)</text>
      </svg>
    </div>
  );
}
