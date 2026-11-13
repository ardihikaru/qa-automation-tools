import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Area,
  Bar,
  BarChart,
  CartesianGrid,
  ComposedChart,
  Legend,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { AlertTriangle, ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';
import type {
  CostByFeature,
  CostByModel,
  CostEfficiency,
  CostFilterOptions,
  CostGateway,
  CostOverview,
  CostRecent,
  CostTrendPoint,
} from '../utils/costTypes';

const PAGE_SIZE = 20;
const PRESETS = [7, 30, 90] as const;

// ---------- helpers ----------

class ApiError extends Error {}

async function getJSON<T>(path: string, params: URLSearchParams | null, signal: AbortSignal): Promise<T> {
  const qs = params && params.toString() ? `?${params.toString()}` : '';
  const res = await fetch(`${path}${qs}`, { credentials: 'include', signal });
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiError(
      (body && typeof body.error === 'string' && body.error) || `Request failed (${res.status})`,
    );
  }
  return res.json();
}

function isoDay(d: Date): string {
  return d.toISOString().slice(0, 10); // UTC
}

function rangeForDays(days: number): { from: string; to: string } {
  const to = new Date();
  const from = new Date(to.getTime() - (days - 1) * 86400000);
  return { from: isoDay(from), to: isoDay(to) };
}

function currentMonth(): string {
  return new Date().toISOString().slice(0, 7);
}

const usd = (n: number | undefined | null, digits = 4) => `$${(n ?? 0).toFixed(digits)}`;
const idr = (n: number | undefined | null) => `Rp ${Math.round(n ?? 0).toLocaleString('id-ID')}`;
const num = (n: number | undefined | null) => (n ?? 0).toLocaleString();
const ms = (n: number | undefined | null) => {
  const v = n ?? 0;
  return v >= 1000 ? `${(v / 1000).toFixed(2)} s` : `${Math.round(v)} ms`;
};

function relTime(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return '';
  const s = Math.round((Date.now() - t) / 1000);
  if (s < 60) return 'just now';
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return `${Math.floor(s / 86400)} d ago`;
}

function absTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

function ErrorBanner({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div role="alert" className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">
      <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1 break-words">{message}</div>
      <button type="button" onClick={onRetry} className="btn btn-secondary !py-1 !px-3 text-sm shrink-0">
        Retry
      </button>
    </div>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <div className="flex items-center justify-center h-full min-h-[6rem] text-sm text-gray-500 text-center px-4">{children}</div>;
}

function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`animate-pulse bg-gray-100 rounded ${className}`} aria-hidden="true" />;
}

function StatusBadge({ status }: { status: string }) {
  const ok = status === 'ok' || status === 'success';
  return <span className={`badge ${ok ? 'badge-success' : 'badge-error'}`}>{ok ? 'ok' : status || 'error'}</span>;
}

function Card({ label, children, sub, loading }: { label: string; children: React.ReactNode; sub?: React.ReactNode; loading: boolean }) {
  return (
    <div className="card p-4 min-w-0">
      <div className="text-sm text-gray-500">{label}</div>
      {loading ? (
        <>
          <Skeleton className="h-8 w-24 mt-1" />
          <Skeleton className="h-3 w-32 mt-2" />
        </>
      ) : (
        <>
          <div className="text-2xl font-bold truncate">{children}</div>
          {sub && <div className="text-xs text-gray-500 mt-1">{sub}</div>}
        </>
      )}
    </div>
  );
}

// Generic section loader with stale-response protection.
function useCostData<T>(path: string, params: URLSearchParams | null, key: string, reloadToken: number) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [retry, setRetry] = useState(0);
  const paramsRef = useRef(params);
  paramsRef.current = params;

  useEffect(() => {
    const ctrl = new AbortController();
    setLoading(true);
    setError(null);
    getJSON<T>(path, paramsRef.current, ctrl.signal)
      .then((d) => {
        if (!ctrl.signal.aborted) setData(d);
      })
      .catch((e) => {
        if (ctrl.signal.aborted) return;
        setData(null);
        setError(e instanceof Error ? e.message : 'Request failed');
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setLoading(false);
      });
    return () => ctrl.abort();
  }, [path, key, reloadToken, retry]);

  return { data, error, loading, retry: () => setRetry((n) => n + 1) };
}

// ---------- view ----------

export default function CostAnalytics() {
  const [days, setDays] = useState<number | null>(30);
  const [range, setRange] = useState(() => rangeForDays(30));
  const [feature, setFeature] = useState('');
  const [provider, setProvider] = useState('');
  const [model, setModel] = useState('');
  const [status, setStatus] = useState('');
  const [reload, setReload] = useState(0);

  const params = useMemo(() => {
    const p = new URLSearchParams({ from: range.from, to: range.to });
    if (feature) p.set('feature', feature);
    if (provider) p.set('provider', provider);
    if (model) p.set('model', model);
    if (status) p.set('status', status);
    return p;
  }, [range, feature, provider, model, status]);
  const key = params.toString();

  const overview = useCostData<CostOverview>('/api/cost/overview', params, key, reload);
  const trend = useCostData<CostTrendPoint[]>('/api/cost/trend', params, key, reload);
  const byFeature = useCostData<CostByFeature[]>('/api/cost/by-feature', params, key, reload);
  const byModel = useCostData<CostByModel[]>('/api/cost/by-model', params, key, reload);
  const efficiency = useCostData<CostEfficiency>('/api/cost/efficiency', params, key, reload);
  const filters = useCostData<CostFilterOptions>('/api/cost/filters', null, '', reload);

  const applyPreset = (d: number) => {
    setDays(d);
    setRange(rangeForDays(d));
  };
  const setCustom = (k: 'from' | 'to', v: string) => {
    if (!v) return;
    setDays(null);
    setRange((r) => ({ ...r, [k]: v }));
  };
  const refresh = () => {
    if (days) setRange(rangeForDays(days));
    setReload((n) => n + 1);
  };

  const ov = overview.data;
  const emptyRange = !!ov && ov.total_requests === 0;
  const summaryErrors = [
    ['Overview', overview],
    ['Trend', trend],
    ['By feature', byFeature],
    ['By model', byModel],
    ['Efficiency', efficiency],
  ] as const;
  const failed = summaryErrors.filter(([, s]) => s.error);
  const anyLoading = overview.loading || trend.loading || byFeature.loading || byModel.loading;

  const modelData = useMemo(
    () =>
      (byModel.data ?? []).slice(0, 10).map((m) => ({
        name: `${m.provider}/${m.model}`,
        cost: m.cost,
        calls: m.calls,
      })),
    [byModel.data],
  );
  const featureData = useMemo(() => (byFeature.data ?? []).slice(0, 10), [byFeature.data]);

  const selectCls = 'rounded-md border border-gray-300 bg-white px-2 py-1.5 text-sm max-w-[12rem] min-w-0';

  return (
    <div className="space-y-6 min-w-0">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold text-gray-900">Cost Analytics</h2>
          <p className="text-gray-600">AI usage, spend, errors and latency. Days are UTC.</p>
        </div>
        <button
          type="button"
          onClick={refresh}
          disabled={anyLoading}
          aria-busy={anyLoading}
          className="btn btn-secondary inline-flex items-center gap-2"
        >
          <RefreshCw className={`w-4 h-4 ${anyLoading ? 'animate-spin' : ''}`} aria-hidden="true" />
          Refresh
        </button>
      </div>

      {/* Filters */}
      <div className="card p-4 flex flex-wrap items-end gap-4">
        <div role="group" aria-label="Date range presets" className="flex rounded-md border border-gray-300 overflow-hidden">
          {PRESETS.map((d) => (
            <button
              key={d}
              type="button"
              aria-pressed={days === d}
              onClick={() => applyPreset(d)}
              className={`px-3 py-1.5 text-sm ${days === d ? 'bg-primary-600 text-white' : 'bg-white text-gray-700 hover:bg-gray-50'}`}
            >
              {d} days
            </button>
          ))}
        </div>
        <label className="text-xs text-gray-500 flex flex-col gap-1">
          From
          <input type="date" value={range.from} max={range.to} onChange={(e) => setCustom('from', e.target.value)} className={selectCls} />
        </label>
        <label className="text-xs text-gray-500 flex flex-col gap-1">
          To
          <input type="date" value={range.to} min={range.from} onChange={(e) => setCustom('to', e.target.value)} className={selectCls} />
        </label>
        <label className="text-xs text-gray-500 flex flex-col gap-1">
          Feature
          <select value={feature} onChange={(e) => setFeature(e.target.value)} className={selectCls}>
            <option value="">All features</option>
            {(filters.data?.features ?? []).map((f) => <option key={f} value={f}>{f}</option>)}
          </select>
        </label>
        <label className="text-xs text-gray-500 flex flex-col gap-1">
          Provider
          <select value={provider} onChange={(e) => setProvider(e.target.value)} className={selectCls}>
            <option value="">All providers</option>
            {(filters.data?.providers ?? []).map((f) => <option key={f} value={f}>{f}</option>)}
          </select>
        </label>
        <label className="text-xs text-gray-500 flex flex-col gap-1">
          Model
          <select value={model} onChange={(e) => setModel(e.target.value)} className={selectCls}>
            <option value="">All models</option>
            {(filters.data?.models ?? []).map((f) => <option key={f} value={f}>{f}</option>)}
          </select>
        </label>
        <label className="text-xs text-gray-500 flex flex-col gap-1">
          Status
          <select value={status} onChange={(e) => setStatus(e.target.value)} className={selectCls}>
            <option value="">All</option>
            <option value="ok">ok</option>
            <option value="error">error</option>
          </select>
        </label>
      </div>
      {filters.error && <ErrorBanner message={`Filter options: ${filters.error}`} onRetry={filters.retry} />}

      {failed.map(([name, s]) => (
        <ErrorBanner key={name} message={`${name}: ${s.error}`} onRetry={s.retry} />
      ))}

      {emptyRange && !anyLoading && (
        <div className="card p-4 text-sm text-gray-600">No AI calls recorded in this range.</div>
      )}

      {/* Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        <Card label="Calls" loading={overview.loading} sub={`${num(efficiency.data?.success ?? ov?.success_calls)} succeeded`}>
          {num(ov?.total_requests)}
        </Card>
        <Card
          label="Success rate"
          loading={overview.loading}
          sub={`${num(ov?.error_calls)} error${ov?.error_calls === 1 ? '' : 's'}`}
        >
          {ov && ov.total_requests > 0 ? `${((1 - ov.error_rate) * 100).toFixed(1)}%` : '-'}
        </Card>
        <Card
          label="Total cost"
          loading={overview.loading}
          sub={
            <>
              {idr(ov?.total_cost_idr)}
              {!!ov?.estimated_cost && ov.estimated_cost > 0 && (
                <span title="Notional pay-as-you-go estimate, not real spend">
                  {' '}
                  (incl. {usd(ov.estimated_cost)} estimated, not real spend)
                </span>
              )}
            </>
          }
        >
          <span className="text-green-600">{usd(ov?.total_cost)}</span>
        </Card>
        <Card
          label="Tokens"
          loading={overview.loading}
          sub={`${num(ov?.input_tokens)} in / ${num(ov?.output_tokens)} out`}
        >
          {num(ov?.total_tokens)}
        </Card>
        <Card label="Avg latency" loading={overview.loading} sub={`p95 ${ms(ov?.p95_latency_ms)}`}>
          {ms(ov?.avg_latency_ms)}
        </Card>
        <Card label="Fallbacks" loading={overview.loading} sub="Calls served by a fallback provider">
          {num(efficiency.data?.fallbacks ?? ov?.fallback_calls)}
        </Card>
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <div className="card min-w-0 xl:col-span-2">
          <div className="card-header"><h3 className="font-semibold">Daily cost, calls and errors</h3></div>
          <div className="card-body h-72">
            {trend.loading ? (
              <Skeleton className="h-full" />
            ) : trend.data && trend.data.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <ComposedChart data={trend.data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="date" tick={{ fontSize: 10 }} tickFormatter={(v: string) => v.slice(5)} />
                  <YAxis yAxisId="cost" tick={{ fontSize: 10 }} tickFormatter={(v: number) => `$${v.toFixed(2)}`} />
                  <YAxis yAxisId="calls" orientation="right" allowDecimals={false} tick={{ fontSize: 10 }} />
                  <Tooltip
                    formatter={(v: number, name: string) => (name === 'Cost (USD)' ? [usd(v), name] : [v, name])}
                  />
                  <Legend />
                  <Area yAxisId="cost" type="monotone" dataKey="cost" name="Cost (USD)" stroke="#3b82f6" fill="#3b82f6" fillOpacity={0.15} strokeWidth={2} />
                  <Line yAxisId="calls" type="monotone" dataKey="calls" name="Calls" stroke="#10b981" strokeWidth={2} dot={false} />
                  <Line yAxisId="calls" type="monotone" dataKey="errors" name="Errors" stroke="#ef4444" strokeWidth={2} dot={{ r: 2 }} />
                </ComposedChart>
              </ResponsiveContainer>
            ) : (
              <Empty>{trend.error ? 'Trend unavailable' : 'No AI calls recorded in this range'}</Empty>
            )}
          </div>
        </div>

        <div className="card min-w-0">
          <div className="card-header"><h3 className="font-semibold">Cost by feature (top 10)</h3></div>
          <div className="card-body h-80">
            {byFeature.loading ? (
              <Skeleton className="h-full" />
            ) : featureData.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={featureData} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis type="number" tick={{ fontSize: 10 }} tickFormatter={(v: number) => `$${v.toFixed(2)}`} />
                  <YAxis type="category" dataKey="feature" width={130} tick={{ fontSize: 10 }} />
                  <Tooltip formatter={(v: number) => [usd(v), 'Cost']} />
                  <Bar dataKey="cost" fill="#3b82f6" />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <Empty>{byFeature.error ? 'Unavailable' : 'No AI calls recorded in this range'}</Empty>
            )}
          </div>
        </div>

        <div className="card min-w-0">
          <div className="card-header"><h3 className="font-semibold">Cost by provider / model (top 10)</h3></div>
          <div className="card-body h-80">
            {byModel.loading ? (
              <Skeleton className="h-full" />
            ) : modelData.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={modelData} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis type="number" tick={{ fontSize: 10 }} tickFormatter={(v: number) => `$${v.toFixed(2)}`} />
                  <YAxis type="category" dataKey="name" width={150} tick={{ fontSize: 10 }} />
                  <Tooltip formatter={(v: number, name: string) => (name === 'Cost' ? [usd(v), name] : [v, name])} />
                  <Bar dataKey="cost" name="Cost" fill="#8b5cf6" />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <Empty>{byModel.error ? 'Unavailable' : 'No AI calls recorded in this range'}</Empty>
            )}
          </div>
        </div>
      </div>

      <RecentCalls params={params} filterKey={key} reload={reload} />
      <Gateway reload={reload} />
    </div>
  );
}

// ---------- recent calls ----------

function RecentCalls({ params, filterKey, reload }: { params: URLSearchParams; filterKey: string; reload: number }) {
  const [offset, setOffset] = useState(0);
  const [failedOnly, setFailedOnly] = useState(false);
  const [expanded, setExpanded] = useState<string | number | null>(null);

  // Reset to page 1 whenever filters or the toggle change.
  useEffect(() => setOffset(0), [filterKey, failedOnly]);

  const p = useMemo(() => {
    const q = new URLSearchParams(params);
    if (failedOnly) q.set('status', 'error');
    q.set('limit', String(PAGE_SIZE));
    q.set('offset', String(offset));
    return q;
  }, [params, failedOnly, offset]);
  const { data, error, loading, retry } = useCostData<CostRecent>('/api/cost/recent', p, p.toString(), reload);

  const items = data?.items ?? [];
  const total = data?.total ?? 0;
  const last = Math.min(offset + PAGE_SIZE, total);

  return (
    <section className="card min-w-0" aria-labelledby="recent-h">
      <div className="card-header flex flex-wrap items-center justify-between gap-3">
        <h3 id="recent-h" className="font-semibold">Recent calls</h3>
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input type="checkbox" checked={failedOnly} onChange={(e) => setFailedOnly(e.target.checked)} />
          Failed only
        </label>
      </div>
      <div className="card-body space-y-3">
        {error && <ErrorBanner message={error} onRetry={retry} />}
        <div className="overflow-x-auto">
          <table className="min-w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase text-gray-500 border-b">
                <th className="py-2 pr-4">Time</th>
                <th className="py-2 pr-4">Status</th>
                <th className="py-2 pr-4">Feature</th>
                <th className="py-2 pr-4">Model</th>
                <th className="py-2 pr-4 text-right">Tokens in/out</th>
                <th className="py-2 pr-4 text-right">Cost</th>
                <th className="py-2 pr-4 text-right">Latency</th>
                <th className="py-2 pr-4">Fallback</th>
                <th className="py-2">Error</th>
              </tr>
            </thead>
            <tbody>
              {loading && items.length === 0 &&
                Array.from({ length: 5 }).map((_, i) => (
                  <tr key={i}><td colSpan={9} className="py-2"><Skeleton className="h-5" /></td></tr>
                ))}
              {!loading && items.length === 0 && !error && (
                <tr><td colSpan={9} className="py-8 text-center text-gray-500">
                  {failedOnly ? 'No failed calls in this range' : 'No AI calls recorded in this range'}
                </td></tr>
              )}
              {items.map((r) => (
                <tr key={r.id} className={`border-b align-top ${loading ? 'opacity-60' : ''}`}>
                  <td className="py-2 pr-4 whitespace-nowrap" title={absTime(r.created_at)}>
                    <div>{relTime(r.created_at)}</div>
                    <div className="text-xs text-gray-500">{absTime(r.created_at)}</div>
                  </td>
                  <td className="py-2 pr-4"><StatusBadge status={r.status} /></td>
                  <td className="py-2 pr-4 max-w-[12rem] truncate" title={`${r.feature}${r.caller ? ` (${r.caller})` : ''}`}>{r.feature}</td>
                  <td className="py-2 pr-4 max-w-[14rem] truncate" title={`${r.provider}/${r.model}`}>{r.provider}/{r.model}</td>
                  <td className="py-2 pr-4 text-right whitespace-nowrap">{num(r.input_tokens)} / {num(r.output_tokens)}</td>
                  <td className="py-2 pr-4 text-right whitespace-nowrap" title={idr(r.cost_idr)}>{usd(r.cost)}</td>
                  <td className="py-2 pr-4 text-right whitespace-nowrap">{ms(r.latency_ms)}</td>
                  <td className="py-2 pr-4">{r.fallback ? <span className="badge badge-warning">fallback</span> : <span className="text-gray-400">-</span>}</td>
                  <td className="py-2 max-w-[20rem]">
                    {r.error ? (
                      <button
                        type="button"
                        aria-expanded={expanded === r.id}
                        onClick={() => setExpanded(expanded === r.id ? null : r.id)}
                        title={r.error}
                        className={`text-left text-red-700 ${expanded === r.id ? 'whitespace-pre-wrap break-words' : 'truncate block max-w-full'}`}
                      >
                        {r.error}
                      </button>
                    ) : (
                      <span className="text-gray-400">-</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="flex items-center justify-between text-sm text-gray-600">
          <span>{total > 0 ? `${offset + 1}-${last} of ${num(total)}` : '0 results'}</span>
          <div className="flex gap-2">
            <button type="button" className="btn btn-secondary !px-2 !py-1" aria-label="Previous page" disabled={loading || offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>
              <ChevronLeft className="w-4 h-4" aria-hidden="true" />
            </button>
            <button type="button" className="btn btn-secondary !px-2 !py-1" aria-label="Next page" disabled={loading || offset + PAGE_SIZE >= total} onClick={() => setOffset(offset + PAGE_SIZE)}>
              <ChevronRight className="w-4 h-4" aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}

// ---------- ai-manager gateway ----------

function Gateway({ reload }: { reload: number }) {
  const [month, setMonth] = useState(currentMonth());
  const [all, setAll] = useState(false);
  const p = useMemo(() => new URLSearchParams({ month: all ? 'all' : month || currentMonth() }), [month, all]);
  const { data, error, loading, retry } = useCostData<CostGateway>('/api/cost/gateway', p, p.toString(), reload);

  const vendors = data?.usage?.vendors ?? [];
  const gwCalls = data?.usage?.total_calls ?? 0;
  const gwErrors = data?.usage?.total_errors ?? 0;
  const gwReal = vendors.reduce((s, v) => s + (v.cost_usd - v.estimated_cost_usd), 0);
  const gwEst = vendors.reduce((s, v) => s + v.estimated_cost_usd, 0);
  const gwRealIdr = vendors.reduce((s, v) => s + (v.cost_idr - v.estimated_cost_idr), 0);

  return (
    <section className="card min-w-0" aria-labelledby="gw-h">
      <div className="card-header flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 id="gw-h" className="font-semibold">ai-manager gateway</h3>
          <p className="text-xs text-gray-500">ai-manager is shared with other apps, so gateway totals can exceed this app's local totals.</p>
        </div>
        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-sm text-gray-700">
            <input type="checkbox" checked={all} onChange={(e) => setAll(e.target.checked)} />
            All time
          </label>
          <label className="text-xs text-gray-500 flex items-center gap-2">
            Month
            <input
              type="month"
              value={month}
              disabled={all}
              onChange={(e) => e.target.value && setMonth(e.target.value)}
              className="rounded-md border border-gray-300 px-2 py-1.5 text-sm disabled:opacity-50"
            />
          </label>
        </div>
      </div>
      <div className="card-body space-y-5">
        {error && <ErrorBanner message={error} onRetry={retry} />}
        {loading && !data && <Skeleton className="h-32" />}

        {data && !data.available && (
          <div role="status" className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 break-words">
            <strong>Gateway data unavailable.</strong> {data.error || 'ai-manager could not be reached.'}
          </div>
        )}

        {data && (data.local || data.available) && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="rounded-md border border-gray-200 p-3">
              <div className="text-xs uppercase text-gray-500">This app (local ledger)</div>
              {data.local ? (
                <dl className="mt-2 text-sm grid grid-cols-2 gap-y-1">
                  <dt className="text-gray-500">Calls</dt><dd className="text-right">{num(data.local.calls)}</dd>
                  <dt className="text-gray-500">Errors</dt><dd className="text-right">{num(data.local.errors)}</dd>
                  <dt className="text-gray-500">Cost</dt><dd className="text-right">{usd(data.local.cost)} <span className="text-xs text-gray-500">({idr(data.local.cost_idr)})</span></dd>
                </dl>
              ) : <div className="mt-2 text-sm text-gray-500">No local data</div>}
            </div>
            <div className="rounded-md border border-gray-200 p-3">
              <div className="text-xs uppercase text-gray-500">Gateway (all apps){data.usage ? `, ${data.usage.period}` : ''}</div>
              {data.available && data.usage ? (
                <dl className="mt-2 text-sm grid grid-cols-2 gap-y-1">
                  <dt className="text-gray-500">Calls</dt><dd className="text-right">{num(gwCalls)}</dd>
                  <dt className="text-gray-500">Errors</dt><dd className="text-right">{num(gwErrors)}</dd>
                  <dt className="text-gray-500">Real spend</dt><dd className="text-right">{usd(gwReal)} <span className="text-xs text-gray-500">({idr(gwRealIdr)})</span></dd>
                  <dt className="text-gray-500">Estimated (notional)</dt><dd className="text-right">{usd(gwEst)}</dd>
                </dl>
              ) : <div className="mt-2 text-sm text-gray-500">Unavailable</div>}
            </div>
          </div>
        )}

        {data?.available && (
          <>
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <caption className="text-left font-medium mb-2">By vendor and model</caption>
                <thead>
                  <tr className="text-left text-xs uppercase text-gray-500 border-b">
                    <th className="py-2 pr-4">Vendor</th>
                    <th className="py-2 pr-4">Model</th>
                    <th className="py-2 pr-4 text-right">Calls</th>
                    <th className="py-2 pr-4 text-right">Errors</th>
                    <th className="py-2 pr-4 text-right">Tokens in/out</th>
                    <th className="py-2 pr-4 text-right">Real cost</th>
                    <th className="py-2 text-right">Estimated</th>
                  </tr>
                </thead>
                <tbody>
                  {vendors.length === 0 && (
                    <tr><td colSpan={7} className="py-6 text-center text-gray-500">No gateway usage for this period</td></tr>
                  )}
                  {vendors.map((v, i) => (
                    <tr key={`${v.vendor}-${v.model}-${i}`} className="border-b">
                      <td className="py-2 pr-4 max-w-[12rem] truncate" title={`${v.vendor} (${v.vendor_type})`}>{v.vendor}</td>
                      <td className="py-2 pr-4 max-w-[14rem] truncate" title={v.model}>{v.model}</td>
                      <td className="py-2 pr-4 text-right">{num(v.calls)}</td>
                      <td className="py-2 pr-4 text-right">{num(v.errors)}</td>
                      <td className="py-2 pr-4 text-right whitespace-nowrap">{num(v.input_tokens)} / {num(v.output_tokens)}</td>
                      <td className="py-2 pr-4 text-right whitespace-nowrap" title={idr(v.cost_idr - v.estimated_cost_idr)}>{usd(v.cost_usd - v.estimated_cost_usd)}</td>
                      <td className="py-2 text-right whitespace-nowrap text-gray-500" title={idr(v.estimated_cost_idr)}>{usd(v.estimated_cost_usd)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <caption className="text-left font-medium mb-2">Recent gateway calls</caption>
                <thead>
                  <tr className="text-left text-xs uppercase text-gray-500 border-b">
                    <th className="py-2 pr-4">Time</th>
                    <th className="py-2 pr-4">Status</th>
                    <th className="py-2 pr-4">Vendor / model</th>
                    <th className="py-2 pr-4 text-right">Tokens in/out</th>
                    <th className="py-2 pr-4 text-right">Cost</th>
                    <th className="py-2 pr-4 text-right">Latency</th>
                    <th className="py-2 pr-4">Client</th>
                    <th className="py-2">Error</th>
                  </tr>
                </thead>
                <tbody>
                  {(data.recent ?? []).length === 0 && (
                    <tr><td colSpan={8} className="py-6 text-center text-gray-500">No recent gateway calls</td></tr>
                  )}
                  {(data.recent ?? []).map((r, i) => (
                    <tr key={`${r.ts}-${i}`} className="border-b align-top">
                      <td className="py-2 pr-4 whitespace-nowrap" title={absTime(r.ts)}>
                        <div>{relTime(r.ts)}</div>
                        <div className="text-xs text-gray-500">{absTime(r.ts)}</div>
                      </td>
                      <td className="py-2 pr-4">
                        <StatusBadge status={r.status} />
                        {r.fallback && <span className="badge badge-warning ml-1">fallback</span>}
                      </td>
                      <td className="py-2 pr-4 max-w-[16rem] truncate" title={`${r.vendor}/${r.model} (requested ${r.requested_provider})`}>{r.vendor}/{r.model}</td>
                      <td className="py-2 pr-4 text-right whitespace-nowrap">{num(r.input_tokens)} / {num(r.output_tokens)}</td>
                      <td className="py-2 pr-4 text-right whitespace-nowrap" title={`${idr(r.cost_idr)} (${r.cost_source})`}>{usd(r.cost_usd)}</td>
                      <td className="py-2 pr-4 text-right whitespace-nowrap">{ms(r.latency_ms)}</td>
                      <td className="py-2 pr-4 whitespace-nowrap">{r.client_ip}</td>
                      <td className="py-2 max-w-[18rem] truncate text-red-700" title={r.error}>{r.error || <span className="text-gray-400">-</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </div>
    </section>
  );
}
