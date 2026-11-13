// /assistants/:productId/:code: one synced agent, skill, rule, command or instructions file.
// The body is rendered with the local safe markdown renderer, or shown as source.
import { ReactNode, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import { AlertTriangle, ArrowLeft, ExternalLink } from 'lucide-react';
import { useAuth } from '../../auth/AuthContext';
import { ApiError } from '../products/api';
import { Annotation, CODE_RE, canWriteKnowledge } from '../knowledge/api';
import { useLoad } from '../knowledge/hooks';
import { ErrorNote, RelTime } from '../knowledge/ui';
import AnnotationEditor from './AnnotationEditor';
import Markdown from './Markdown';
import { AssistantDetail, Frontmatter, KIND_META, getAssistant, isAssistantKind, useProducts } from './api';
import { EmptyState, KindBadge, QuietTag, Segmented } from './ui';

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-3 px-4 py-4 first:pt-4">
      <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
      {children}
    </section>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[7rem_minmax(0,1fr)] gap-3 py-1.5">
      <dt className="text-sm text-gray-500">{label}</dt>
      <dd className="text-sm text-gray-800 min-w-0 break-words">{children}</dd>
    </div>
  );
}

const MONO_KEYS = new Set(['paths', 'globs', 'applyTo', 'argument-hint']);

function FmValue({ k, v }: { k: string; v: unknown }) {
  if (v === null || v === undefined || v === '') return <span className="text-gray-400">empty</span>;
  if (Array.isArray(v) && v.every((x) => ['string', 'number', 'boolean'].includes(typeof x))) {
    if (!v.length) return <span className="text-gray-400">none</span>;
    return (
      <span className="flex flex-wrap gap-1">
        {v.map((x, i) => (
          <QuietTag key={i} mono={MONO_KEYS.has(k)} title={String(x)}>
            {String(x)}
          </QuietTag>
        ))}
      </span>
    );
  }
  if (typeof v === 'string') {
    // Comma lists of tools read better as tags.
    if (/tools$/i.test(k) && v.includes(',')) return <FmValue k={k} v={v.split(',').map((s) => s.trim()).filter(Boolean)} />;
    return <span className={`whitespace-pre-wrap ${MONO_KEYS.has(k) ? 'font-mono text-xs' : ''}`}>{v}</span>;
  }
  if (typeof v === 'number' || typeof v === 'boolean') return <span className="tabular-nums">{String(v)}</span>;
  return <pre className="overflow-x-auto rounded bg-gray-50 p-2 font-mono text-xs text-gray-700">{JSON.stringify(v, null, 2)}</pre>;
}

function FrontmatterList({ fm, skip }: { fm: Frontmatter | null; skip: Record<string, string> }) {
  const entries = Object.entries(fm ?? {}).filter(([k, v]) => !(k in skip && typeof v === 'string' && v.trim() === skip[k].trim()));
  if (!entries.length) {
    return <p className="text-sm text-gray-500">{fm && Object.keys(fm).length ? 'Only the name and description, shown above.' : 'This file has no frontmatter.'}</p>;
  }
  return (
    <dl className="divide-y divide-gray-100">
      {entries.map(([k, v]) => (
        <Fact key={k} label={k}>
          <FmValue k={k} v={v} />
        </Fact>
      ))}
    </dl>
  );
}

function DetailSkeleton() {
  return (
    <div aria-hidden="true" className="space-y-4 motion-safe:animate-pulse">
      <div className="h-7 w-72 rounded bg-gray-200" />
      <div className="h-4 w-1/2 rounded bg-gray-100" />
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
        <div className="card h-96" />
        <div className="card h-64" />
      </div>
    </div>
  );
}

export default function AssistantDetailView() {
  const params = useParams();
  const location = useLocation();
  const { user } = useAuth();
  const { products } = useProducts();
  const pid = Number(params.productId);
  const code = params.code ?? '';
  const valid = Number.isInteger(pid) && pid > 0 && CODE_RE.test(code);

  const [annotation, setAnnotation] = useState<{ key: string; value: Annotation | null } | null>(null);
  const [mode, setMode] = useState<'rendered' | 'source'>('rendered');

  const key = `${pid}:${code}`;
  const { data, loading, error, reload } = useLoad<AssistantDetail | null>(valid ? key : 'invalid', (signal) =>
    valid
      ? getAssistant(pid, code, signal).catch((e) => {
          // A 404 (unknown product or code) is a state of the page, not an error.
          if (e instanceof ApiError && e.status === 404) return null;
          throw e;
        })
      : Promise.resolve(null),
  );
  const notFound = !valid || (!loading && !error && data === null);

  const from = (location.state as { from?: string } | null)?.from;
  const kindFromCode = code.split('~')[0];
  const kind = data?.kind ?? (isAssistantKind(kindFromCode) ? kindFromCode : null);
  const backTo = from || (kind ? KIND_META[kind].listPath : '/agents');
  const backLabel = kind ? `Back to ${kind === 'agent' ? 'agents' : kind === 'skill' ? 'repo skills' : 'rules'}` : 'Back';

  const productName = products.find((p) => p.id === pid)?.name;
  const currentAnnotation = annotation?.key === key ? annotation.value : (data?.annotation ?? null);

  const back = (
    <Link to={backTo} className="inline-flex items-center gap-1.5 text-sm text-gray-600 hover:text-gray-900">
      <ArrowLeft className="w-4 h-4" aria-hidden="true" />
      {backLabel}
    </Link>
  );

  if (notFound) {
    return (
      <div className="space-y-4 min-w-0">
        {back}
        <div className="card">
          <EmptyState title="This file was not found">
            <p>It may have been removed from the automation repo before the last sync, or the link is wrong.</p>
          </EmptyState>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="space-y-4 min-w-0">
        {back}
        {error ? <ErrorNote onRetry={reload}>{error}</ErrorNote> : <DetailSkeleton />}
      </div>
    );
  }

  const description = data.description?.trim();
  const fmSkip: Record<string, string> = { name: data.name ?? '', description: description ?? '' };
  const canWrite = !!user && canWriteKnowledge(user.role);

  return (
    <div className="space-y-6 min-w-0" aria-busy={loading}>
      {back}

      <header className="space-y-2 min-w-0">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex flex-wrap items-center gap-2 min-w-0">
              <h2 className="text-2xl font-bold text-gray-900 break-words min-w-0">{data.name || data.path}</h2>
              <KindBadge kind={data.kind} />
            </div>
            <p className="text-sm text-gray-600">
              {productName ? (
                <Link to={`/products?product=${pid}`} className="hover:underline underline-offset-2">
                  {productName}
                </Link>
              ) : (
                `Product #${pid}`
              )}
              <span className="mx-2 text-gray-300" aria-hidden="true">
                /
              </span>
              <span className="font-mono text-xs break-all">{data.path}</span>
            </p>
          </div>
          {data.source_url && (
            <a href={data.source_url} target="_blank" rel="noopener noreferrer" className="btn btn-secondary text-sm inline-flex items-center gap-1.5 shrink-0">
              <ExternalLink className="w-4 h-4" aria-hidden="true" />
              View in repo
            </a>
          )}
        </div>
        {description && <p className="text-gray-700 max-w-[85ch] break-words">{description}</p>}
      </header>

      {data.removed_at && (
        <div role="status" className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" aria-hidden="true" />
          <p className="min-w-0">
            This file is no longer in the automation repo. It was removed
            {data.removed_sync_id ? ` in sync #${data.removed_sync_id}` : ''} (<RelTime at={data.removed_at} />). Below is the last synced copy.
          </p>
        </div>
      )}

      {error && <ErrorNote onRetry={reload}>{error}</ErrorNote>}

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_24rem] items-start">
        <section className="card min-w-0" aria-label="File content">
          <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 border-b border-gray-200">
            <h3 className="font-semibold text-gray-900">Content</h3>
            <Segmented<'rendered' | 'source'>
              label="Show content as"
              value={mode}
              onChange={setMode}
              options={[
                { value: 'rendered', label: 'Formatted' },
                { value: 'source', label: 'Source' },
              ]}
            />
          </div>
          <div className="px-4 py-5 sm:px-6 min-w-0">
            {mode === 'rendered' ? (
              <Markdown source={data.body ?? ''} base={data.source_url} />
            ) : data.body ? (
              <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-[13px] leading-relaxed text-gray-800">{data.body}</pre>
            ) : (
              <p className="text-sm text-gray-500">This file has no content after its frontmatter.</p>
            )}
          </div>
        </section>

        <aside className="card min-w-0 divide-y divide-gray-200">
          <Section title="Details">
            <dl className="divide-y divide-gray-100">
              <Fact label="Kind">{KIND_META[data.kind]?.label ?? data.kind}</Fact>
              <Fact label="Product">{productName ?? `#${pid}`}</Fact>
              <Fact label="Path">
                <span className="font-mono text-xs break-all">{data.path}</span>
              </Fact>
              <Fact label="Code">
                <span className="font-mono text-xs break-all">{data.code}</span>
              </Fact>
              {data.changed_sync_id ? <Fact label="Last changed">sync #{data.changed_sync_id}</Fact> : null}
            </dl>
          </Section>
          <Section title="Frontmatter">
            <FrontmatterList fm={data.frontmatter} skip={fmSkip} />
          </Section>
          <Section title="Notes">
            <AnnotationEditor
              key={key}
              pid={pid}
              code={data.code}
              annotation={currentAnnotation}
              canWrite={canWrite}
              onSaved={(a) => setAnnotation({ key, value: a })}
            />
          </Section>
        </aside>
      </div>
    </div>
  );
}
