// Shared layout of every knowledge detail page: breadcrumbs, removed banner, header with one
// coverage sentence, then the main column and a side column with details and notes.
import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { ExternalLink, Lock, Pencil } from 'lucide-react';
import { Coverage, EntityBase, TYPE_LABEL, knowledgeHref } from '../api';
import { coverageSentence } from '../coverage';
import { AnnotationEditor } from '../EntityPanel';
import { LocalBadge } from '../ui';
import { Crumb, Crumbs, Fields, StatusBar } from './kit';
import { PageCtx, baseCrumbs } from './data';


interface Props {
  ctx: PageCtx;
  entity: EntityBase;
  title: ReactNode;
  /** Plain-text title for tooltips (defaults to the code). */
  titleText?: string;
  crumbs: Crumb[];
  coverage?: Coverage | null;
  /** Chips next to the repo link (e.g. the release status). */
  meta?: ReactNode;
  /** Lines under the header (e.g. linked features). */
  related?: ReactNode;
  details?: [string, ReactNode][];
  /** Knowledge view tab that holds this draft type, for the "Edit draft" link. */
  draftTab?: string;
  /** The title already is the code (test cases): don't repeat it above. */
  codeIsTitle?: boolean;
  children: ReactNode;
}

export default function EntityShell({ ctx, entity: e, title, titleText, crumbs, coverage, meta, related, details, draftTab, codeIsTitle, children }: Props) {
  const removed = !!e.removed_at;
  const local = e.origin === 'local';
  return (
    <div className="space-y-6">
      <Crumbs items={[...baseCrumbs(ctx), ...crumbs]} />

      {removed && (
        <div role="status" className="rounded-lg border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-800">
          Removed from the repo{e.removed_sync_id ? ` in sync #${e.removed_sync_id}` : ''}. It stays here for history and no longer counts towards coverage.
        </div>
      )}

      <header className="space-y-2 min-w-0">
        <p className="flex flex-wrap items-center gap-2 text-sm text-gray-500">
          <span>{TYPE_LABEL[e.type]}</span>
          {!codeIsTitle && <span className={`font-mono ${removed ? 'line-through' : 'text-gray-700'}`}>{e.code}</span>}
          {local && <LocalBadge />}
        </p>
        <h1 className={`text-2xl font-semibold leading-snug break-words ${removed ? 'line-through text-gray-400' : 'text-gray-900'}`} title={titleText}>
          {title}
        </h1>
        {coverage !== undefined && (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
            <StatusBar coverage={coverage} className="w-48" />
            <p className="text-sm text-gray-700 tabular-nums">{coverageSentence(coverage)}</p>
          </div>
        )}
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
          {meta}
          {e.source_url && (
            <a href={e.source_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary-600 hover:underline min-h-8">
              <ExternalLink className="w-4 h-4" aria-hidden="true" /> View in repo
            </a>
          )}
          {!local && (
            <span className="inline-flex items-center gap-1.5 text-gray-500 min-h-8">
              <Lock className="w-4 h-4" aria-hidden="true" /> Managed by the repo, edit it there and sync
            </span>
          )}
          {local && draftTab && ctx.canWrite && (
            <Link to={`${knowledgeHref(ctx.pid, draftTab)}&e=${e.type}:${encodeURIComponent(e.code)}`} className="inline-flex items-center gap-1 text-primary-600 hover:underline min-h-8">
              <Pencil className="w-4 h-4" aria-hidden="true" /> Edit draft
            </Link>
          )}
        </div>
        {e.source_file && !e.source_url && (
          <p className="text-xs text-gray-400 font-mono break-all">
            {e.source_file}
            {e.source_line ? `:${e.source_line}` : ''}
          </p>
        )}
        {related}
      </header>

      <div className="grid gap-8 xl:grid-cols-[minmax(0,1fr)_20rem] 2xl:grid-cols-[minmax(0,1fr)_24rem] items-start border-t border-gray-200 pt-6">
        <div className="min-w-0 space-y-8">{children}</div>
        <aside className="min-w-0 space-y-6 xl:border-l xl:border-gray-200 xl:pl-6" aria-label="Details and notes">
          {details && details.some(([, v]) => v) && (
            <section className="space-y-3">
              <h2 className="text-base font-semibold text-gray-900">Details</h2>
              <Fields rows={details} />
            </section>
          )}
          <section className="space-y-3">
            <h2 className="text-base font-semibold text-gray-900">Notes and tags</h2>
            <AnnotationEditor key={`${e.type}:${e.code}`} pid={ctx.pid} type={e.type} code={e.code} annotation={e.annotation} canWrite={ctx.canWrite} onSaved={ctx.onAnnotation} />
          </section>
        </aside>
      </div>
    </div>
  );
}
