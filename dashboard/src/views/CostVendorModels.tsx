import { Fragment, useEffect, useState } from 'react';
import { Area, AreaChart, ResponsiveContainer, Tooltip, XAxis } from 'recharts';
import { AlertTriangle, ChevronDown, ChevronRight } from 'lucide-react';
import type { CostByVendor, CostTrendPoint, CostVendor, CostVendorModel } from '../utils/costTypes';

const usd = (n: number, digits = 4) => `$${n.toFixed(digits)}`;
const idr = (n: number) => `Rp ${Math.round(n).toLocaleString('id-ID')}`;
const num = (n: number) => n.toLocaleString();

// The ledger's feature name for ID to EN knowledge translation calls.
const FEATURE_LABELS: Record<string, string> = { translation: 'Translation (knowledge, ID to EN)' };
const featureLabel = (f: string) => FEATURE_LABELS[f] ?? f;

function ShareBar({ pct }: { pct: number }) {
  return (
    <div className="flex items-center gap-2 min-w-[7rem]">
      <div className="h-2 flex-1 rounded bg-gray-100 overflow-hidden" role="presentation">
        <div className="h-full bg-primary-600" style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} />
      </div>
      <span className="text-xs text-gray-600 tabular-nums w-12 text-right">{pct.toFixed(1)}%</span>
    </div>
  );
}

// Compact daily-cost trend for one model: the trend endpoint accepts the same filters plus model.
function ModelTrend({ params, model }: { params: URLSearchParams; model: string }) {
  const [pts, setPts] = useState<CostTrendPoint[] | null>(null);
  const [failed, setFailed] = useState(false);
  const key = `${params.toString()}|${model}`;
  useEffect(() => {
    const ctrl = new AbortController();
    const q = new URLSearchParams(params);
    q.set('model', model);
    setPts(null);
    setFailed(false);
    fetch(`/api/cost/trend?${q.toString()}`, { credentials: 'include', signal: ctrl.signal })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error('trend'))))
      .then((d: CostTrendPoint[]) => setPts(d))
      .catch(() => {
        if (!ctrl.signal.aborted) setFailed(true);
      });
    return () => ctrl.abort();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  if (failed) return <div className="text-xs text-gray-500">Trend unavailable</div>;
  if (!pts) return <div className="h-16 animate-pulse bg-gray-100 rounded" aria-hidden="true" />;
  return (
    <div className="h-16" aria-label={`Daily cost trend for ${model}`}>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={pts}>
          <XAxis dataKey="date" hide />
          <Tooltip
            formatter={(v: number) => [usd(v), 'Cost']}
            labelFormatter={(l: string) => l}
            contentStyle={{ fontSize: 11 }}
          />
          <Area type="monotone" dataKey="cost" stroke="#3b82f6" fill="#3b82f6" fillOpacity={0.15} strokeWidth={1.5} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}

function ModelDetail({ m, params }: { m: CostVendorModel; params: URLSearchParams }) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 bg-gray-50 px-4 py-3">
      <div>
        <div className="text-xs uppercase text-gray-500 mb-1">By feature</div>
        <ul className="text-sm space-y-1">
          {m.features.map((f) => (
            <li key={f.feature} className="flex flex-wrap justify-between gap-x-3">
              <span className={f.feature === 'translation' ? 'font-medium text-primary-700' : ''}>{featureLabel(f.feature)}</span>
              <span className="text-gray-600 tabular-nums">
                {num(f.calls)} calls, {num(f.input_tokens)} in / {num(f.output_tokens)} out, {usd(f.cost)} ({idr(f.cost_idr)})
              </span>
            </li>
          ))}
        </ul>
      </div>
      <div>
        <div className="text-xs uppercase text-gray-500 mb-1">Daily cost ({m.model || 'unknown'})</div>
        <ModelTrend params={params} model={m.model} />
      </div>
    </div>
  );
}

function VendorRow({ v, params }: { v: CostVendor; params: URLSearchParams }) {
  const [open, setOpen] = useState(true);
  const [openModel, setOpenModel] = useState<string | null>(null);
  return (
    <Fragment>
      <tr className="bg-gray-50 border-b">
        <td className="py-2 px-3 font-semibold">
          <button
            type="button"
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
            className="inline-flex items-center gap-1"
          >
            {open ? <ChevronDown className="w-4 h-4" aria-hidden="true" /> : <ChevronRight className="w-4 h-4" aria-hidden="true" />}
            {v.vendor}
            <span className="ml-1 text-xs font-normal text-gray-500">
              {v.models.length} model{v.models.length === 1 ? '' : 's'}
            </span>
          </button>
        </td>
        <td className="py-2 px-3 text-right tabular-nums">{num(v.calls)}</td>
        <td className="py-2 px-3 text-right tabular-nums">{num(v.input_tokens)} / {num(v.output_tokens)}</td>
        <td className="py-2 px-3 text-right tabular-nums font-semibold">{usd(v.cost)}<div className="text-xs font-normal text-gray-500">{idr(v.cost_idr)}</div></td>
        <td className="py-2 px-3 text-right tabular-nums">{usd(v.avg_cost_per_call)}</td>
        <td className="py-2 px-3"><ShareBar pct={v.share_pct} /></td>
      </tr>
      {open &&
        v.models.map((m) => {
          const id = `${m.provider}/${m.model}`;
          const isOpen = openModel === id;
          return (
            <Fragment key={id}>
              <tr className="border-b hover:bg-gray-50">
                <td className="py-2 pl-8 pr-3">
                  <button
                    type="button"
                    onClick={() => setOpenModel(isOpen ? null : id)}
                    aria-expanded={isOpen}
                    className="inline-flex items-center gap-1 text-left"
                    title={m.model}
                  >
                    {isOpen ? <ChevronDown className="w-3.5 h-3.5" aria-hidden="true" /> : <ChevronRight className="w-3.5 h-3.5" aria-hidden="true" />}
                    <span className="font-medium">{m.label}</span>
                    <span className="text-xs text-gray-500 break-all">{m.model !== m.label ? m.model : ''}</span>
                    {m.features.some((f) => f.feature === 'translation') && (
                      <span className="ml-1 rounded bg-primary-50 px-1.5 py-0.5 text-xs text-primary-700">translation</span>
                    )}
                  </button>
                </td>
                <td className="py-2 px-3 text-right tabular-nums">
                  {num(m.calls)}
                  {m.errors > 0 && <div className="text-xs text-red-600">{num(m.errors)} failed</div>}
                </td>
                <td className="py-2 px-3 text-right tabular-nums">{num(m.input_tokens)} / {num(m.output_tokens)}</td>
                <td className="py-2 px-3 text-right tabular-nums">{usd(m.cost)}<div className="text-xs text-gray-500">{idr(m.cost_idr)}</div></td>
                <td className="py-2 px-3 text-right tabular-nums">{usd(m.avg_cost_per_call)}</td>
                <td className="py-2 px-3"><ShareBar pct={m.share_pct} /></td>
              </tr>
              {isOpen && (
                <tr className="border-b">
                  <td colSpan={6} className="p-0"><ModelDetail m={m} params={params} /></td>
                </tr>
              )}
            </Fragment>
          );
        })}
    </Fragment>
  );
}

export default function CostVendorModels({
  data,
  loading,
  error,
  onRetry,
  params,
}: {
  data: CostByVendor | null;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  params: URLSearchParams;
}) {
  const vendors = data?.vendors ?? [];
  return (
    <section className="card min-w-0" aria-labelledby="vendor-h">
      <div className="card-header">
        <h3 id="vendor-h" className="font-semibold">By vendor &amp; model</h3>
        <p className="text-xs text-gray-500 mt-1">
          Vendor is derived from the model id. Share is of total cost in the range. Expand a model for its features (including
          translation) and daily trend.
        </p>
      </div>
      <div className="card-body">
        {error ? (
          <div role="alert" className="flex items-start gap-3 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800">
            <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" aria-hidden="true" />
            <div className="min-w-0 flex-1 break-words">By vendor: {error}</div>
            <button type="button" onClick={onRetry} className="btn btn-secondary !py-1 !px-3 text-sm shrink-0">Retry</button>
          </div>
        ) : loading ? (
          <div className="animate-pulse bg-gray-100 rounded h-32" aria-hidden="true" />
        ) : vendors.length === 0 ? (
          <div className="text-sm text-gray-500 text-center py-6">No AI calls recorded in this range</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-sm">
              <thead>
                <tr className="text-left text-xs uppercase text-gray-500 border-b">
                  <th className="py-2 px-3">Vendor / model</th>
                  <th className="py-2 px-3 text-right">Calls</th>
                  <th className="py-2 px-3 text-right">Tokens in / out</th>
                  <th className="py-2 px-3 text-right">Cost</th>
                  <th className="py-2 px-3 text-right">Avg / call</th>
                  <th className="py-2 px-3">Share</th>
                </tr>
              </thead>
              <tbody>
                {vendors.map((v) => <VendorRow key={v.vendor} v={v} params={params} />)}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}
