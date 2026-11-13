import { FormEvent, useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import {
  ApiError,
  EntityDetail,
  Enums,
  Epic,
  LOCAL_CODE_RE,
  NEEDS_REASON,
  StoryDetail,
  TestCaseDetail,
  errorText,
  listStories,
  saveDraft,
} from './api';
import { inputCls, selectCls } from './styles';

export type DraftKind = 'story' | 'test_case';

interface Props {
  pid: number;
  kind: DraftKind;
  /** Editing an existing local draft; absent for a new one. */
  existing?: StoryDetail | TestCaseDetail;
  enums: Enums;
  epics: Epic[];
  /** `{code, name}` of active features, for the story's feature links. */
  features: { code: string; name: string }[];
  onClose: () => void;
  onSaved: (e: EntityDetail) => void;
}

const labelCls = 'block text-sm font-medium text-gray-700 mb-1';
const areaCls =
  'w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent';

export default function DraftModal({ pid, kind, existing, enums, epics, features, onClose, onSaved }: Props) {
  const editing = !!existing;
  const story = existing?.type === 'story' ? (existing as StoryDetail) : undefined;
  const tc = existing?.type === 'test_case' ? (existing as TestCaseDetail) : undefined;
  const activeEpics = epics.filter((e) => !e.removed_at);

  const [code, setCode] = useState(existing?.code ?? 'L-');
  const [epic, setEpic] = useState(existing ? (existing as StoryDetail | TestCaseDetail).epic_code : activeEpics[0]?.code ?? '');
  // story
  const [text, setText] = useState(story?.text ?? '');
  const [featureCodes, setFeatureCodes] = useState<string[]>(story?.feature_codes ?? []);
  const [featureText, setFeatureText] = useState('');
  // test case
  const [storyCode, setStoryCode] = useState(tc?.story_code ?? '');
  const [scenario, setScenario] = useState(tc?.scenario ?? 'Given \nWhen \nThen ');
  const [polarity, setPolarity] = useState(tc?.polarity ?? enums.polarity[0] ?? '+');
  const [level, setLevel] = useState(tc?.level ?? enums.level[0] ?? '');
  const [priority, setPriority] = useState(tc?.priority ?? enums.priority[1] ?? enums.priority[0] ?? '');
  const [status, setStatus] = useState(tc?.status ?? 'planned');
  const [specs, setSpecs] = useState((tc?.spec_paths ?? []).join('\n'));
  const [reasonCat, setReasonCat] = useState(tc?.reason_category ?? '');
  const [reasonDetail, setReasonDetail] = useState(tc?.reason_detail ?? '');
  const [storyOptions, setStoryOptions] = useState<{ code: string; text: string }[]>([]);

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inflight = useRef(false);
  const formRef = useRef<HTMLFormElement>(null);

  // Move focus into the dialog (the code field, or the first enabled field when editing), so
  // Escape and Tab act on the modal rather than on the drawer behind it.
  useEffect(() => {
    formRef.current?.querySelector<HTMLElement>('input:not([disabled]), select:not([disabled]), textarea:not([disabled])')?.focus();
  }, []);

  // Story suggestions for a test case: the active stories of the chosen epic.
  useEffect(() => {
    if (kind !== 'test_case' || !epic) return;
    const ac = new AbortController();
    listStories(pid, { epic, page_size: 200 }, ac.signal)
      .then((p) => setStoryOptions(p.items.map((s) => ({ code: s.code, text: s.text }))))
      .catch(() => !ac.signal.aborted && setStoryOptions([]));
    return () => ac.abort();
  }, [pid, kind, epic]);

  function addFeature() {
    const f = featureText.trim();
    if (!f) return;
    if (!featureCodes.includes(f)) setFeatureCodes([...featureCodes, f]);
    setFeatureText('');
  }

  const specList = specs
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean);
  const needsReason = NEEDS_REASON.includes(status);

  function problem(): string | null {
    if (!editing && !LOCAL_CODE_RE.test(code)) return 'Code must look like L-ABC-1 (L- then up to 30 of A–Z, 0–9 and -)';
    if (!epic) return 'Pick an epic';
    if (kind === 'story') {
      if (!text.trim()) return 'Story text is required';
      return null;
    }
    if (!scenario.trim()) return 'Scenario is required';
    if (specList.length > 20) return 'At most 20 spec paths';
    if (specList.some((s) => s.length > 300)) return 'A spec path is at most 300 characters';
    if (status === 'automated' && specList.length === 0) return 'An automated case needs at least one spec path';
    if (needsReason && (!reasonCat || !reasonDetail.trim())) return `Status ${status} needs a reason category and detail`;
    if ([...reasonDetail].length > 1000) return 'Reason detail is at most 1000 characters';
    return null;
  }

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (inflight.current) return;
    const p = problem();
    if (p) return setError(p);
    inflight.current = true;
    setSaving(true);
    setError(null);
    try {
      const body =
        kind === 'story'
          ? { code, epic_code: epic, text: text.trim(), feature_codes: featureCodes }
          : {
              code,
              epic_code: epic,
              story_code: storyCode.trim(),
              scenario: scenario.trim(),
              polarity,
              level,
              priority,
              status,
              spec_paths: specList,
              reason_category: reasonCat,
              reason_detail: reasonDetail.trim(),
            };
      const saved = await saveDraft(pid, kind, body, editing ? existing!.code : undefined);
      onSaved(saved);
    } catch (err) {
      setError(err instanceof ApiError && err.status === 409 ? err.message : errorText(err, 'Could not save'));
    } finally {
      inflight.current = false;
      setSaving(false);
    }
  }

  const title = `${editing ? 'Edit' : 'New'} local ${kind === 'story' ? 'story' : 'test case'}${editing ? `: ${existing!.code}` : ''}`;
  const featureName = new Map(features.map((f) => [f.code, f.name]));

  return (
    <Modal title={title} onClose={() => !saving && onClose()}>
      <form ref={formRef} onSubmit={submit} className="space-y-3" noValidate>
        <p className="text-xs text-gray-500">Local drafts live only in this app and never collide with synced ids. Add them to the automation repo to make them real.</p>
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block">
            <span className={labelCls}>Code</span>
            <input
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              disabled={editing}
              maxLength={32}
              className={`${inputCls} w-full font-mono disabled:bg-gray-50`}
              aria-invalid={!editing && code.length > 2 && !LOCAL_CODE_RE.test(code)}
            />
            <span className="block text-xs text-gray-500 mt-1">L- then A–Z, 0–9 or -</span>
          </label>
          <label className="block">
            <span className={labelCls}>Epic</span>
            <select value={epic} onChange={(e) => setEpic(e.target.value)} className={`${selectCls} w-full`}>
              {activeEpics.length === 0 && <option value="">No epics (sync first)</option>}
              {activeEpics.map((e) => (
                <option key={e.code} value={e.code}>
                  {e.code}
                  {e.title ? ` — ${e.title.length > 60 ? `${e.title.slice(0, 60)}…` : e.title}` : ''}
                </option>
              ))}
            </select>
          </label>
        </div>

        {kind === 'story' ? (
          <>
            <label className="block">
              <span className={labelCls}>Story</span>
              <textarea value={text} onChange={(e) => setText(e.target.value)} rows={4} className={areaCls} placeholder="As a … I want … so that …" />
            </label>
            <div>
              <span className={labelCls}>Features</span>
              <div className="flex gap-2">
                <input
                  value={featureText}
                  onChange={(e) => setFeatureText(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault();
                      addFeature();
                    }
                  }}
                  list="kb-draft-features"
                  placeholder="A01.07"
                  aria-label="Feature code"
                  className={`${inputCls} flex-1 font-mono`}
                />
                <button type="button" className="btn btn-secondary text-sm" onClick={addFeature} disabled={!featureText.trim()}>
                  Add
                </button>
                <datalist id="kb-draft-features">
                  {features.map((f) => (
                    <option key={f.code} value={f.code}>
                      {f.name}
                    </option>
                  ))}
                </datalist>
              </div>
              {featureCodes.length > 0 && (
                <ul className="mt-2 flex flex-wrap gap-1.5">
                  {featureCodes.map((f) => (
                    <li key={f} className="badge bg-gray-100 text-gray-800 gap-1 max-w-full" title={featureName.get(f) ?? 'Unknown feature'}>
                      <span className="font-mono truncate">{f}</span>
                      <button type="button" aria-label={`Remove ${f}`} onClick={() => setFeatureCodes(featureCodes.filter((x) => x !== f))}>
                        <X className="w-3 h-3" aria-hidden="true" />
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </>
        ) : (
          <>
            <label className="block">
              <span className={labelCls}>Story (optional)</span>
              <input value={storyCode} onChange={(e) => setStoryCode(e.target.value)} list="kb-draft-stories" className={`${inputCls} w-full font-mono`} placeholder="US-OUTREG-05" />
              <datalist id="kb-draft-stories">
                {storyOptions.map((s) => (
                  <option key={s.code} value={s.code}>
                    {s.text.slice(0, 80)}
                  </option>
                ))}
              </datalist>
            </label>
            <label className="block">
              <span className={labelCls}>Scenario (Given / When / Then)</span>
              <textarea value={scenario} onChange={(e) => setScenario(e.target.value)} rows={4} className={`${areaCls} font-mono`} />
            </label>
            <div className="grid gap-3 grid-cols-2 sm:grid-cols-4">
              {(
                [
                  ['Polarity', polarity, setPolarity, enums.polarity],
                  ['Level', level, setLevel, enums.level],
                  ['Priority', priority, setPriority, enums.priority],
                  ['Status', status, setStatus, enums.status],
                ] as [string, string, (v: string) => void, string[]][]
              ).map(([label, v, set, opts]) => (
                <label key={label} className="block">
                  <span className={labelCls}>{label}</span>
                  <select value={v} onChange={(e) => set(e.target.value)} className={`${selectCls} w-full`}>
                    {opts.map((o) => (
                      <option key={o} value={o}>
                        {o}
                      </option>
                    ))}
                  </select>
                </label>
              ))}
            </div>
            <label className="block">
              <span className={labelCls}>Spec paths {status === 'automated' && <span className="text-red-600">*</span>}</span>
              <textarea value={specs} onChange={(e) => setSpecs(e.target.value)} rows={2} className={`${areaCls} font-mono`} placeholder="tests/outreg/register.spec.ts (one per line)" />
            </label>
            <div className="grid gap-3 sm:grid-cols-[12rem_1fr]">
              <label className="block">
                <span className={labelCls}>Reason {needsReason && <span className="text-red-600">*</span>}</span>
                <select value={reasonCat} onChange={(e) => setReasonCat(e.target.value)} className={`${selectCls} w-full`}>
                  <option value="">—</option>
                  {enums.reason_category.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              </label>
              <label className="block">
                <span className={labelCls}>Reason detail</span>
                <input value={reasonDetail} onChange={(e) => setReasonDetail(e.target.value)} maxLength={1000} className={`${inputCls} w-full`} />
              </label>
            </div>
          </>
        )}

        {error && (
          <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2 break-words">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-2 pt-2">
          <button type="button" className="btn btn-secondary" disabled={saving} onClick={onClose}>
            Cancel
          </button>
          <SubmitButton type="submit" className="btn btn-primary" loading={saving} loadingLabel="Saving…">
            {editing ? 'Save' : 'Create draft'}
          </SubmitButton>
        </div>
      </form>
    </Modal>
  );
}
