import { FormEvent, KeyboardEvent, ReactNode, useEffect, useRef, useState } from 'react';
import { EvidenceList, SyncStamp } from './EvidenceList';
import { Link } from 'react-router-dom';
import { Bot, ExternalLink, Lock, Maximize2, Pencil, Trash2, X } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import {
  Annotation,
  EntityDetail,
  EntityType,
  EpicDetail,
  FeatureDetail,
  ModuleDetail,
  PartDetail,
  Ref,
  StoryDetail,
  SubmoduleDetail,
  TYPE_LABEL,
  TestCase,
  TestCaseDetail,
  deleteAnnotation,
  deleteDraft,
  entityHref,
  errorText,
  getEntity,
  isAbort,
  periodText,
  putAnnotation,
  relativeTime,
} from './api';
import { CaseStatusChip, CodeLabel, CoverageBar, ErrorNote, Loading, LocalBadge, ReleaseChip, RemovedBadge } from './ui';

export interface Target {
  type: EntityType;
  code: string;
}

interface Props {
  pid: number;
  target: Target;
  canWrite: boolean;
  /** Bumped by the parent after a draft edit so the panel reloads. */
  reloadKey: number;
  onOpen: (t: Target) => void;
  onEditDraft: (e: StoryDetail | TestCaseDetail) => void;
  /** A draft was deleted or an annotation changed: lists may be stale. */
  onMutated: (deleted?: boolean) => void;
}

// ---------------------------------------------------------------- small pieces

function Section({ title, children, count }: { title: string; children: ReactNode; count?: number }) {
  return (
    <section className="space-y-2">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
        {title}
        {count !== undefined && <span className="ml-1 font-normal normal-case tracking-normal">({count})</span>}
      </h3>
      {children}
    </section>
  );
}

function Fields({ rows }: { rows: [string, ReactNode][] }) {
  const shown = rows.filter(([, v]) => v !== '' && v !== null && v !== undefined && v !== false);
  if (shown.length === 0) return null;
  return (
    <dl className="grid grid-cols-[8rem_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-sm">
      {shown.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-gray-500">{k}</dt>
          <dd className="text-gray-900 break-words min-w-0">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

function RefButton({ type, r, onOpen, children }: { type: EntityType; r: Ref | { code: string; origin: string; removed_at: string | null; name?: string }; onOpen: (t: Target) => void; children?: ReactNode }) {
  return (
    <button
      type="button"
      onClick={() => onOpen({ type, code: r.code })}
      className="kb-row w-full text-left flex items-start gap-2 rounded-md px-2 py-1.5 min-h-8 min-w-0"
      title={r.name || r.code}
    >
      <CodeLabel code={r.code} origin={r.origin} removed={!!r.removed_at} />
      {r.name && <span className={`text-sm truncate min-w-0 ${r.removed_at ? 'text-gray-400 line-through' : 'text-gray-700'}`}>{r.name}</span>}
      {children}
    </button>
  );
}

function RefList({ type, refs, onOpen, empty }: { type: EntityType; refs: Ref[]; onOpen: (t: Target) => void; empty: string }) {
  if (!refs?.length) return <p className="text-sm text-gray-500">{empty}</p>;
  return (
    <ul className="max-h-80 overflow-y-auto -mx-2">
      {refs.map((r) => (
        <li key={r.code}>
          <RefButton type={type} r={r} onOpen={onOpen} />
        </li>
      ))}
    </ul>
  );
}

const KEYWORD = /^(Given|When|Then|And|But)\b/;

/** Text with markdown `backticks` rendered as inline code and **double stars** as bold. */
export function InlineCode({ text }: { text: string }) {
  const parts = text.split(/(`[^`]+`|\*\*[^*]+\*\*)/g);
  return (
    <>
      {parts.map((p, i) =>
        p.length > 4 && p.startsWith('**') && p.endsWith('**') ? (
          <strong key={i} className="font-semibold">
            {p.slice(2, -2)}
          </strong>
        ) : p.length > 2 && p.startsWith('`') && p.endsWith('`') ? (
          <code key={i} className="rounded bg-gray-100 px-1 py-px font-mono text-[0.85em] text-gray-800 break-all">
            {p.slice(1, -1)}
          </code>
        ) : (
          p
        ),
      )}
    </>
  );
}

/** Given/When/Then text: `<br>` and newlines become lines, the leading keyword is bold. */
export function Scenario({ text }: { text: string }) {
  const lines = text
    .replace(/<br\s*\/?>/gi, '\n')
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean);
  return (
    <div className="text-sm text-gray-800 space-y-0.5 break-words">
      {lines.map((l, i) => {
        const m = l.match(KEYWORD);
        return (
          <p key={i}>
            {m ? (
              <>
                <strong className="font-semibold text-gray-900">{m[1]}</strong>
                <InlineCode text={l.slice(m[1].length)} />
              </>
            ) : (
              <InlineCode text={l} />
            )}
          </p>
        );
      })}
    </div>
  );
}

function SpecPaths({ paths }: { paths: string[] | null }) {
  if (!paths?.length) return null;
  return (
    <ul className="space-y-0.5">
      {paths.map((p) => (
        <li key={p} className="font-mono text-xs text-gray-600 break-all">
          {p}
        </li>
      ))}
    </ul>
  );
}

function CaseRow({ c, onOpen }: { c: TestCase; onOpen: (t: Target) => void }) {
  return (
    <li className="rounded-md border border-gray-100 px-2 py-1.5 space-y-1">
      <RefButton type="test_case" r={{ ...c, name: c.scenario.replace(/<br\s*\/?>/gi, ' ') }} onOpen={onOpen}>
        <span className="ml-auto shrink-0 inline-flex items-center gap-1">
          {c.automated && <Bot className="w-3.5 h-3.5 text-green-600" aria-label="automated" />}
          <CaseStatusChip status={c.status} />
        </span>
      </RefButton>
      {c.spec_paths && c.spec_paths.length > 0 && (
        <div className="pl-4">
          <SpecPaths paths={c.spec_paths} />
        </div>
      )}
    </li>
  );
}

// ---------------------------------------------------------------- annotation editor

function TagInput({ tags, onChange, disabled }: { tags: string[]; onChange: (t: string[]) => void; disabled: boolean }) {
  const [text, setText] = useState('');
  const [problem, setProblem] = useState<string | null>(null);
  function add() {
    const t = text.trim().replace(/,$/, '').trim();
    if (!t) return;
    if ([...t].length > 40) return setProblem('A tag is at most 40 characters');
    if (tags.length >= 20) return setProblem('At most 20 tags');
    setProblem(null);
    if (!tags.includes(t)) onChange([...tags, t]);
    setText('');
  }
  function onKey(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      add();
    } else if (e.key === 'Backspace' && !text && tags.length) {
      onChange(tags.slice(0, -1));
    }
  }
  return (
    <div>
      <div className="flex flex-wrap items-center gap-1.5 rounded-lg border border-gray-300 px-2 py-1.5 focus-within:ring-2 focus-within:ring-primary-500">
        {tags.map((t) => (
          <span key={t} className="badge bg-gray-100 text-gray-800 gap-1 max-w-full">
            <span className="truncate" title={t}>
              {t}
            </span>
            {!disabled && (
              <button type="button" className="text-gray-500 hover:text-gray-800" aria-label={`Remove tag ${t}`} onClick={() => onChange(tags.filter((x) => x !== t))}>
                <X className="w-3 h-3" aria-hidden="true" />
              </button>
            )}
          </span>
        ))}
        {!disabled && (
          <input
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKey}
            onBlur={add}
            placeholder={tags.length ? '' : 'Add a tag, press Enter'}
            aria-label="Add tag"
            className="flex-1 min-w-[8rem] text-sm outline-none py-0.5"
          />
        )}
      </div>
      {problem && <p className="text-xs text-red-700 mt-1">{problem}</p>}
    </div>
  );
}

export function AnnotationEditor({ pid, type, code, annotation, canWrite, onSaved }: { pid: number; type: EntityType; code: string; annotation: Annotation | null; canWrite: boolean; onSaved: (a: Annotation | null) => void }) {
  const [note, setNote] = useState(annotation?.note ?? '');
  const [tags, setTags] = useState<string[]>(annotation?.tags ?? []);
  const [busy, setBusy] = useState<'save' | 'clear' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const inflight = useRef(false);
  const dirty = note !== (annotation?.note ?? '') || tags.join('\u0000') !== (annotation?.tags ?? []).join('\u0000');

  async function act(kind: 'save' | 'clear') {
    if (inflight.current) return;
    inflight.current = true;
    setBusy(kind);
    setError(null);
    setSaved(false);
    try {
      if (kind === 'save') {
        const a = await putAnnotation(pid, type, code, note, tags);
        onSaved(a);
      } else {
        await deleteAnnotation(pid, type, code);
        setNote('');
        setTags([]);
        onSaved(null);
      }
      setSaved(true);
    } catch (e) {
      setError(errorText(e, 'Could not save the note'));
    } finally {
      inflight.current = false;
      setBusy(null);
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    void act('save');
  }

  if (!canWrite) {
    if (!annotation) return <p className="text-sm text-gray-500">No notes.</p>;
    return (
      <div className="space-y-2">
        {annotation.note && <p className="text-sm text-gray-800 whitespace-pre-wrap break-words">{annotation.note}</p>}
        <TagInput tags={annotation.tags ?? []} onChange={() => undefined} disabled />
      </div>
    );
  }

  return (
    <form onSubmit={submit} className="space-y-2">
      <label className="block">
        <span className="sr-only">Note</span>
        <textarea
          value={note}
          onChange={(e) => {
            setNote(e.target.value);
            setSaved(false);
          }}
          maxLength={10000}
          rows={3}
          placeholder="Local note (kept across syncs)"
          className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
        />
      </label>
      <TagInput
        tags={tags}
        onChange={(t) => {
          setTags(t);
          setSaved(false);
        }}
        disabled={busy !== null}
      />
      {error && <ErrorNote>{error}</ErrorNote>}
      <div className="flex items-center gap-2">
        <SubmitButton type="submit" className="btn btn-primary text-sm py-1.5" loading={busy === 'save'} loadingLabel="Saving…" disabled={busy !== null || !dirty}>
          Save note
        </SubmitButton>
        {annotation && (
          <SubmitButton className="btn btn-secondary text-sm py-1.5" loading={busy === 'clear'} loadingLabel="Clearing…" disabled={busy !== null} onClick={() => void act('clear')}>
            Clear
          </SubmitButton>
        )}
        {saved && !dirty && <span role="status" className="text-xs text-green-700">Saved</span>}
        {annotation?.updated_at && !saved && <span className="text-xs text-gray-400">updated {relativeTime(annotation.updated_at)}</span>}
      </div>
    </form>
  );
}

// ---------------------------------------------------------------- per-type bodies

function Body({ e, onOpen }: { e: EntityDetail; onOpen: (t: Target) => void }) {
  switch (e.type) {
    case 'part': {
      const x = e as PartDetail;
      return (
        <Section title="Modules" count={x.modules?.length}>
          <RefList type="module" refs={x.modules} onOpen={onOpen} empty="No modules." />
        </Section>
      );
    }
    case 'module': {
      const x = e as ModuleDetail;
      return (
        <>
          <Fields rows={[['Part', x.part?.[0] ? <RefButton type="part" r={x.part[0]} onOpen={onOpen} /> : x.part_code]]} />
          <Section title="Sub-modules" count={x.submodules?.length}>
            <RefList type="submodule" refs={x.submodules} onOpen={onOpen} empty="No sub-modules." />
          </Section>
          <Section title="Features" count={x.features?.length}>
            <RefList type="feature" refs={(x.features ?? []).map((f) => ({ code: f.code, name: f.name, origin: f.origin, removed_at: f.removed_at }))} onOpen={onOpen} empty="No features." />
          </Section>
        </>
      );
    }
    case 'submodule': {
      const x = e as SubmoduleDetail;
      return (
        <>
          <Fields rows={[['Module', x.module?.[0] ? <RefButton type="module" r={x.module[0]} onOpen={onOpen} /> : x.module_code]]} />
          <Section title="Features" count={x.features?.length}>
            <RefList type="feature" refs={(x.features ?? []).map((f) => ({ code: f.code, name: f.name, origin: f.origin, removed_at: f.removed_at }))} onOpen={onOpen} empty="No features." />
          </Section>
        </>
      );
    }
    case 'feature': {
      const x = e as FeatureDetail;
      return (
        <>
          <Fields
            rows={[
              ['Release', <ReleaseChip key="r" status={x.release_status} asOf={x.status_as_of} />],
              ['Status as of', x.status_as_of],
              ['Prod since', x.prod_since],
              ['Periode pelaksanaan', periodText(x.dev_period)],
              ['Module', x.module?.[0] ? <RefButton type="module" r={x.module[0]} onOpen={onOpen} /> : x.module_code],
              ['Sub-module', x.submodule?.[0] ? <RefButton type="submodule" r={x.submodule[0]} onOpen={onOpen} /> : x.submodule_code],
              ['Repo entry', x.repo_entry && <span className="font-mono text-xs break-all">{x.repo_entry}</span>],
              ['Evidence', <EvidenceList key="ev" items={x.evidence_items} raw={x.evidence} />],
              ['Last updated', <SyncStamp key="lu" s={x.changed_sync} />],
              ['Notes', x.notes],
              [
                'Coverage',
                x.stats && (
                  <span className="inline-flex flex-wrap items-center gap-3">
                    <CoverageBar value={x.stats.coverage_pct} />
                    <span className="text-xs text-gray-500">
                      {x.stats.automated} of {x.stats.test_cases} test cases automated
                    </span>
                  </span>
                ),
              ],
            ]}
          />
          <Section title="Trace: stories → test cases → specs" count={x.stories?.length}>
            {!x.stories?.length ? (
              <p className="text-sm text-gray-500">No user story references this feature.</p>
            ) : (
              <ul className="space-y-3">
                {x.stories.map((s) => (
                  <li key={s.code} className="rounded-lg border border-gray-200 p-2 space-y-1.5">
                    <RefButton type="story" r={{ code: s.code, name: s.text, origin: s.origin, removed_at: s.removed_at }} onOpen={onOpen} />
                    {s.test_cases?.length ? (
                      <ul className="space-y-1 pl-3">
                        {s.test_cases.map((c) => (
                          <CaseRow key={c.code} c={c} onOpen={onOpen} />
                        ))}
                      </ul>
                    ) : (
                      <p className="pl-5 text-xs text-gray-500">No test cases.</p>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </Section>
        </>
      );
    }
    case 'epic': {
      const x = e as EpicDetail;
      return (
        <>
          <Fields rows={[['Service', x.service], ['Area', x.area]]} />
          <Section title="Stories" count={x.stories?.length}>
            <RefList type="story" refs={(x.stories ?? []).map((s) => ({ code: s.code, name: s.text, origin: s.origin, removed_at: s.removed_at }))} onOpen={onOpen} empty="No stories." />
          </Section>
          <Section title="Test cases" count={x.test_cases?.length}>
            <RefList type="test_case" refs={x.test_cases} onOpen={onOpen} empty="No test cases." />
          </Section>
        </>
      );
    }
    case 'story': {
      const x = e as StoryDetail;
      return (
        <>
          <p className="text-sm text-gray-900 whitespace-pre-wrap break-words">{x.text}</p>
          <Fields
            rows={[
              ['Epic', x.epic?.[0] ? <RefButton type="epic" r={x.epic[0]} onOpen={onOpen} /> : x.epic_code],
              ['Unmapped', x.unmapped_reason],
            ]}
          />
          <Section title="Trace: features" count={x.features?.length}>
            <RefList type="feature" refs={x.features} onOpen={onOpen} empty={x.unmapped_reason ? 'Unmapped: no feature.' : 'No features.'} />
          </Section>
          <Section title="Trace: test cases → specs" count={x.test_cases?.length}>
            {!x.test_cases?.length ? (
              <p className="text-sm text-gray-500">No test cases.</p>
            ) : (
              <ul className="space-y-1">
                {x.test_cases.map((c) => (
                  <CaseRow key={c.code} c={c} onOpen={onOpen} />
                ))}
              </ul>
            )}
          </Section>
        </>
      );
    }
    case 'test_case': {
      const x = e as TestCaseDetail;
      return (
        <>
          <Scenario text={x.scenario} />
          <Fields
            rows={[
              ['Status', <CaseStatusChip key="s" status={x.status} />],
              ['Automated', x.automated ? 'Yes' : 'No'],
              ['Level', x.level],
              ['Priority', x.priority],
              ['Polarity', x.polarity === '+' ? '+ (positive)' : x.polarity === '-' ? '− (negative)' : x.polarity],
              ['Reason', [x.reason_category, x.reason_detail].filter(Boolean).join(': ')],
              ['Epic', x.epic?.[0] ? <RefButton type="epic" r={x.epic[0]} onOpen={onOpen} /> : x.epic_code],
              ['Spec paths', <SpecPaths key="p" paths={x.spec_paths} />],
            ]}
          />
          <Section title="Trace: story">
            {x.story?.[0] ? <RefButton type="story" r={x.story[0]} onOpen={onOpen} /> : <p className="text-sm text-gray-500">{x.story_code ? x.story_code : 'No story.'}</p>}
          </Section>
          <Section title="Trace: features" count={x.features?.length}>
            <RefList type="feature" refs={x.features} onOpen={onOpen} empty="No features." />
          </Section>
        </>
      );
    }
  }
}

function titleOf(e: EntityDetail): string {
  switch (e.type) {
    case 'epic':
      return (e as EpicDetail).title;
    case 'story':
    case 'test_case':
      return '';
    default:
      return (e as PartDetail).name;
  }
}

// ---------------------------------------------------------------- panel

export default function EntityPanel({ pid, target, canWrite, reloadKey, onOpen, onEditDraft, onMutated }: Props) {
  const [entity, setEntity] = useState<EntityDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<{ msg: string; status?: number } | null>(null);
  const [retry, setRetry] = useState(0);
  const [deleting, setDeleting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const inflight = useRef(false);

  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    setActionError(null);
    getEntity(pid, target.type, target.code, ac.signal)
      .then((e) => {
        setEntity(e);
        setLoading(false);
      })
      .catch((e) => {
        if (isAbort(e) || ac.signal.aborted) return;
        setEntity(null);
        setError({ msg: errorText(e, 'Could not load'), status: (e as { status?: number }).status });
        setLoading(false);
      });
    return () => ac.abort();
  }, [pid, target.type, target.code, reloadKey, retry]);

  const showing = entity && entity.type === target.type && entity.code === target.code ? entity : null;

  if (loading && !showing) return <Loading />;
  if (error || !showing) {
    return (
      <div className="space-y-3">
        <p className="font-mono text-sm text-gray-700 break-all">{target.code}</p>
        <ErrorNote onRetry={error?.status === 404 ? undefined : () => setRetry((n) => n + 1)}>
          {error?.status === 404 ? `${TYPE_LABEL[target.type]} ${target.code} was not found.` : error?.msg ?? 'Could not load'}
        </ErrorNote>
      </div>
    );
  }

  const e = showing;
  const draftable = e.type === 'story' || e.type === 'test_case';
  const local = e.origin === 'local';
  const title = titleOf(e);

  async function remove() {
    if (inflight.current || !draftable) return;
    if (!window.confirm(`Delete the local draft ${e.code}? This cannot be undone.`)) return;
    inflight.current = true;
    setDeleting(true);
    setActionError(null);
    try {
      await deleteDraft(pid, e.type as 'story' | 'test_case', e.code);
      onMutated(true);
    } catch (err) {
      setActionError(errorText(err, 'Could not delete'));
    } finally {
      inflight.current = false;
      setDeleting(false);
    }
  }

  return (
    <div className={`space-y-5 ${loading ? 'opacity-60' : ''}`} aria-busy={loading}>
      <header className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs uppercase tracking-wide text-gray-500">{TYPE_LABEL[e.type]}</span>
          {local && <LocalBadge />}
          {e.removed_at && <RemovedBadge syncId={e.removed_sync_id} />}
        </div>
        <h2 className={`font-mono text-lg font-semibold break-all ${e.removed_at ? 'line-through text-gray-400' : 'text-gray-900'}`}>{e.code}</h2>
        {title && <p className="text-base text-gray-800 break-words">{title}</p>}
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
          <Link to={entityHref(pid, e.type, e.code)} className="inline-flex items-center gap-1 text-primary-600 hover:underline min-h-8">
            <Maximize2 className="w-4 h-4" aria-hidden="true" /> Open page
          </Link>
          {e.source_url && (
            <a href={e.source_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary-600 hover:underline min-h-8">
              <ExternalLink className="w-4 h-4" aria-hidden="true" /> View in repo
            </a>
          )}
          {!local && (
            <span className="inline-flex items-center gap-1.5 text-gray-500">
              <Lock className="w-4 h-4" aria-hidden="true" /> Managed by repo — edit there, then Sync
            </span>
          )}
          {local && draftable && canWrite && (
            <span className="inline-flex items-center gap-2">
              <button type="button" className="btn btn-secondary text-sm py-1 min-h-8 inline-flex items-center gap-1" disabled={deleting} onClick={() => onEditDraft(e as StoryDetail | TestCaseDetail)}>
                <Pencil className="w-4 h-4" aria-hidden="true" /> Edit
              </button>
              <SubmitButton className="btn btn-secondary text-sm py-1 min-h-8 text-red-700" loading={deleting} loadingLabel="Deleting…" onClick={remove}>
                <Trash2 className="w-4 h-4" aria-hidden="true" /> Delete
              </SubmitButton>
            </span>
          )}
        </div>
        {e.source_file && !e.source_url && (
          <p className="text-xs text-gray-400 font-mono break-all">
            {e.source_file}
            {e.source_line ? `:${e.source_line}` : ''}
          </p>
        )}
        {actionError && <ErrorNote>{actionError}</ErrorNote>}
      </header>

      <Body e={e} onOpen={onOpen} />

      <Section title="Notes and tags">
        <AnnotationEditor
          key={`${e.type}:${e.code}`}
          pid={pid}
          type={e.type}
          code={e.code}
          annotation={e.annotation}
          canWrite={canWrite}
          onSaved={(a) => {
            setEntity((cur) => (cur && cur.code === e.code ? { ...cur, annotation: a } : cur));
            onMutated();
          }}
        />
      </Section>
    </div>
  );
}
