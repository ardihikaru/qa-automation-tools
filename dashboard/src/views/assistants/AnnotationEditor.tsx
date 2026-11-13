// Notes and tags on a synced assistant file (kb_annotations, entity_type "assistant").
// Mirrors the knowledge EntityPanel editor; kept separate so the two can evolve independently.
import { FormEvent, KeyboardEvent, useRef, useState } from 'react';
import { X } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import { Annotation, errorText, relativeTime } from '../knowledge/api';
import { ErrorNote } from '../knowledge/ui';
import { deleteAssistantAnnotation, putAssistantAnnotation } from './api';

function TagInput({ tags, onChange, disabled, readOnly = false }: { tags: string[]; onChange: (t: string[]) => void; disabled: boolean; readOnly?: boolean }) {
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
  if (readOnly && tags.length === 0) return null;
  return (
    <div>
      <div className={`flex flex-wrap items-center gap-1.5 ${readOnly ? '' : 'rounded-lg border border-gray-300 px-2 py-1.5 focus-within:ring-2 focus-within:ring-primary-500'}`}>
        {tags.map((t) => (
          <span key={t} className="badge bg-gray-100 text-gray-800 gap-1 max-w-full">
            <span className="truncate" title={t}>
              {t}
            </span>
            {!readOnly && (
              <button
                disabled={disabled} type="button" className="text-gray-500 hover:text-gray-800" aria-label={`Remove tag ${t}`} onClick={() => onChange(tags.filter((x) => x !== t))}>
                <X className="w-3 h-3" aria-hidden="true" />
              </button>
            )}
          </span>
        ))}
        {!readOnly && (
          <input
            disabled={disabled}
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

export default function AnnotationEditor({
  pid,
  code,
  annotation,
  canWrite,
  onSaved,
}: {
  pid: number;
  code: string;
  annotation: Annotation | null;
  canWrite: boolean;
  onSaved: (a: Annotation | null) => void;
}) {
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
        onSaved(await putAssistantAnnotation(pid, code, note, tags));
      } else {
        await deleteAssistantAnnotation(pid, code);
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
    if (!annotation || (!annotation.note && !(annotation.tags ?? []).length)) return <p className="text-sm text-gray-500">No notes yet.</p>;
    return (
      <div className="space-y-2">
        {annotation.note && <p className="text-sm text-gray-800 whitespace-pre-wrap break-words">{annotation.note}</p>}
        <TagInput tags={annotation.tags ?? []} onChange={() => undefined} disabled readOnly />
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
          rows={4}
          placeholder="Your note on this file. It stays here across syncs and is not written to the repo."
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
      <div className="flex flex-wrap items-center gap-2">
        <SubmitButton type="submit" className="btn btn-primary text-sm py-1.5" loading={busy === 'save'} loadingLabel="Saving…" disabled={busy !== null || !dirty}>
          Save note
        </SubmitButton>
        {annotation && (
          <SubmitButton className="btn btn-secondary text-sm py-1.5" loading={busy === 'clear'} loadingLabel="Clearing…" disabled={busy !== null} onClick={() => void act('clear')}>
            Clear
          </SubmitButton>
        )}
        {saved && !dirty && (
          <span role="status" className="text-xs text-green-700">
            Saved
          </span>
        )}
        {annotation?.updated_at && !saved && <span className="text-xs text-gray-400">updated {relativeTime(annotation.updated_at)}</span>}
      </div>
    </form>
  );
}
