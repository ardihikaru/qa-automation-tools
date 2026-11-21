import { FormEvent, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronDown, ChevronRight, ExternalLink, FileText, GitMerge, GitPullRequest, Lightbulb, Sparkles } from 'lucide-react';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import { useToast } from '../../components/Toast';
import {
  addFinding,
  answerProposal,
  caseHref,
  checkProposal,
  createProposal,
  errorText,
  Evidence,
  Finding,
  FINDING_STATUS_CLASS,
  FINDING_STATUS_LABEL,
  FindingStatus,
  fmtDate,
  fmtDay,
  getFinding,
  jobHref,
  KIND_LABEL,
  Page,
  ProductOption,
  Proposal,
  ProposalState,
  safePR,
  setFindingStatus,
  STATE_CLASS,
  STATE_LABEL,
  TimelineItem,
} from './api';

const MAX_SELECTED = 10;

// ---------- timeline ----------

const EVENT_TEXT: Record<string, string> = {
  finding_new: 'New finding',
  finding_dismissed: 'Finding dismissed',
  finding_learned: 'Finding learned',
  finding_covered: 'Finding already covered',
  proposal_created: 'Policy proposal started',
  proposal_pr_open: 'Pull request opened',
  proposal_learned: 'Policy merged',
  proposal_covered: 'Already covered by an existing rule',
  proposal_rejected: 'Proposal rejected',
  proposal_closed: 'Pull request closed without merge',
  proposal_failed: 'Proposal failed',
};

function eventIcon(kind: string) {
  if (kind === 'proposal_learned' || kind === 'finding_learned') return <GitMerge className="w-4 h-4 text-green-700" aria-hidden="true" />;
  if (kind === 'proposal_pr_open') return <GitPullRequest className="w-4 h-4 text-indigo-700" aria-hidden="true" />;
  if (kind.startsWith('proposal_')) return <Sparkles className="w-4 h-4 text-blue-700" aria-hidden="true" />;
  return <Lightbulb className="w-4 h-4 text-amber-600" aria-hidden="true" />;
}

export function TimelineTab({ items, productName }: { items: TimelineItem[]; productName: string }) {
  const days = useMemo(() => {
    const out: { day: string; items: TimelineItem[] }[] = [];
    for (const it of items) {
      const day = fmtDay(it.at);
      if (!out.length || out[out.length - 1].day !== day) out.push({ day, items: [] });
      out[out.length - 1].items.push(it);
    }
    return out;
  }, [items]);
  if (!items.length) return <p className="text-sm text-gray-500">Nothing learned yet for {productName}. Findings appear here as reviewers answer and repairs succeed.</p>;
  return (
    <ol className="space-y-5" aria-label="Learning timeline">
      {days.map((d) => (
        <li key={d.day}>
          <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500 mb-2">{d.day}</h3>
          <ul className="space-y-2 border-l border-gray-200 pl-4">
            {d.items.map((it, i) => {
              const pr = safePR(it.pr_url);
              return (
                <li key={`${it.kind}-${it.finding_id ?? it.job_id}-${i}`} className="relative">
                  <span className="absolute -left-[1.6rem] top-0.5 bg-white">{eventIcon(it.kind)}</span>
                  <p className="text-sm text-gray-900 break-words">
                    <span className="font-medium">{EVENT_TEXT[it.kind] ?? it.kind}</span>
                    {it.finding_kind ? <span className="text-gray-500"> · {KIND_LABEL[it.finding_kind]}</span> : null}
                    {it.occurrences && it.occurrences > 1 ? <span className="text-gray-500"> · seen {it.occurrences}×</span> : null}
                  </p>
                  <p className="text-sm text-gray-700 break-words">{it.title}</p>
                  <p className="text-xs text-gray-500 flex flex-wrap gap-x-3 gap-y-1 mt-0.5">
                    <span>{new Date(it.at).toLocaleTimeString(undefined, { timeStyle: 'short' })}</span>
                    <span>{it.product_name}</span>
                    {it.file_path && <code className="break-all">{it.file_path}</code>}
                    {(it.case_codes ?? []).slice(0, 3).map((c) => (
                      <Link key={c} to={caseHref(it.product_id, c)} className="text-primary-700 hover:underline font-mono">
                        {c}
                      </Link>
                    ))}
                    {it.job_id && (
                      <Link to={jobHref(it.job_id)} className="text-primary-700 hover:underline">
                        job
                      </Link>
                    )}
                    {pr && (
                      <a href={pr} target="_blank" rel="noopener noreferrer" className="text-primary-700 hover:underline inline-flex items-center gap-0.5">
                        pull request <ExternalLink className="w-3 h-3" aria-hidden="true" />
                        <span className="sr-only">(opens in a new tab)</span>
                      </a>
                    )}
                  </p>
                </li>
              );
            })}
          </ul>
        </li>
      ))}
    </ol>
  );
}

// ---------- findings ----------

function FindingDetail({ id }: { id: number }) {
  const [ev, setEv] = useState<Evidence[] | null>(null);
  const [err, setErr] = useState('');
  useEffect(() => {
    const ac = new AbortController();
    getFinding(id, ac.signal)
      .then((r) => setEv(r.evidence))
      .catch((e) => !ac.signal.aborted && setErr(errorText(e)));
    return () => ac.abort();
  }, [id]);
  if (err) return <p className="text-sm text-red-700">{err}</p>;
  if (!ev) return <p className="text-sm text-gray-500">Loading evidence…</p>;
  return (
    <ul className="space-y-2" aria-label="Evidence">
      {ev.map((e) => (
        <li key={e.source_key} className="rounded bg-gray-50 px-3 py-2 text-sm">
          <p className="text-xs text-gray-500 flex flex-wrap gap-x-3">
            <span>{fmtDate(e.observed_at)}</span>
            {e.case_code && <span className="font-mono">{e.case_code}</span>}
            {e.job_id && (
              <Link to={jobHref(e.job_id)} className="text-primary-700 hover:underline">
                job {e.job_id}
              </Link>
            )}
            {e.run_ref && <span>run {e.run_ref}</span>}
          </p>
          {e.excerpt && <p className="whitespace-pre-wrap break-words text-gray-800 mt-1">{e.excerpt}</p>}
        </li>
      ))}
    </ul>
  );
}

export function FindingsTab({
  page,
  canWrite,
  allProducts,
  onChanged,
  productName,
}: {
  page: Page<Finding, FindingStatus>;
  canWrite: boolean;
  allProducts: boolean;
  onChanged: () => void;
  productName: string;
}) {
  const toast = useToast();
  const [open, setOpen] = useState<number | null>(null);
  const [selected, setSelected] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  useEffect(() => setSelected((s) => s.filter((id) => page.items.some((f) => f.id === id && f.status === 'new'))), [page]);

  const selProducts = new Set(page.items.filter((f) => selected.includes(f.id)).map((f) => f.product_id));
  const toggle = (f: Finding) =>
    setSelected((s) => (s.includes(f.id) ? s.filter((x) => x !== f.id) : s.length >= MAX_SELECTED ? s : [...s, f.id]));
  const draftProblem =
    selected.length === 0 ? 'Select new findings to draft a policy from.' : selProducts.size > 1 ? 'Select findings of one product only.' : '';

  const draft = async () => {
    if (busy || draftProblem) return;
    setBusy(true);
    try {
      const pid = [...selProducts][0];
      const r = await createProposal(pid, selected);
      toast.success(`Drafting a policy (job ${r.job_id}). It appears under Policies; you approve it before any PR opens.`);
      setSelected([]);
      onChanged();
    } catch (e) {
      toast.error(`Could not start the proposal: ${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const act = async (f: Finding, action: 'dismiss' | 'reopen') => {
    let reason: string | undefined;
    if (action === 'dismiss') {
      const r = window.prompt('Dismiss this finding? Optional reason:', '');
      if (r === null) return;
      reason = r.trim() || undefined;
    }
    try {
      await setFindingStatus(f.id, action, reason);
      onChanged();
    } catch (e) {
      toast.error(errorText(e));
    }
  };

  if (!page.items.length) return <p className="text-sm text-gray-500">No findings for {productName} with these filters.</p>;
  return (
    <div className="space-y-3">
      {canWrite && (
        <div className="flex flex-wrap items-center gap-3 rounded-lg bg-gray-50 px-3 py-2">
          <span className="text-sm text-gray-700">
            {selected.length ? `${selected.length} selected` : 'Select up to 10 new findings of one product on this page'}
          </span>
          <span className="flex-1" />
          {draftProblem && selected.length > 0 && <span className="text-xs text-red-700">{draftProblem}</span>}
          <SubmitButton className="btn btn-primary h-8 px-3 text-sm" loading={busy} loadingLabel="Starting…" disabled={!!draftProblem} onClick={draft}>
            Draft policy{selected.length ? ` from ${selected.length}` : ''}
          </SubmitButton>
        </div>
      )}
      <ul className="divide-y divide-gray-100">
        {page.items.map((f) => {
          const expanded = open === f.id;
          return (
            <li key={f.id} className="py-3">
              <div className="flex items-start gap-3 min-w-0">
                {canWrite && (
                  <input
                    type="checkbox"
                    className="mt-1 h-4 w-4"
                    aria-label={`Select finding ${f.id}`}
                    disabled={f.status !== 'new' || (!selected.includes(f.id) && selected.length >= MAX_SELECTED)}
                    checked={selected.includes(f.id)}
                    onChange={() => toggle(f)}
                  />
                )}
                <button
                  type="button"
                  className="mt-0.5 text-gray-500 hover:text-gray-900"
                  aria-expanded={expanded}
                  aria-label={expanded ? 'Hide evidence' : 'Show evidence'}
                  onClick={() => setOpen(expanded ? null : f.id)}
                >
                  {expanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                </button>
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="text-sm font-medium text-gray-900 break-words">{f.title}</p>
                  <p className="text-xs text-gray-500 flex flex-wrap gap-x-3 gap-y-1">
                    <span className={`badge whitespace-nowrap ${FINDING_STATUS_CLASS[f.status]}`}>{FINDING_STATUS_LABEL[f.status]}</span>
                    <span>{KIND_LABEL[f.kind]}</span>
                    {allProducts && <span>{f.product_name}</span>}
                    <span>seen {f.occurrences}×</span>
                    <span>last {fmtDate(f.last_seen_at)}</span>
                    {f.proposal_job_id && (
                      <Link to={jobHref(f.proposal_job_id)} className="text-primary-700 hover:underline">
                        proposal
                      </Link>
                    )}
                    {f.sources
                      .filter((s) => s.case_code)
                      .slice(0, 3)
                      .map((s) => (
                        <Link key={s.case_code + s.job_id} to={caseHref(f.product_id, s.case_code)} className="text-primary-700 hover:underline font-mono">
                          {s.case_code}
                        </Link>
                      ))}
                  </p>
                  {f.status_note && <p className="text-xs text-gray-500">{f.status_note}</p>}
                  {expanded && (
                    <div className="pt-1 space-y-2">
                      <p className="text-sm text-gray-800 whitespace-pre-wrap break-words">{f.detail}</p>
                      <FindingDetail id={f.id} />
                    </div>
                  )}
                </div>
                {canWrite && (f.status === 'new' || f.status === 'dismissed' || f.status === 'covered') && (
                  <button type="button" className="text-xs text-gray-600 hover:text-gray-900 underline shrink-0" onClick={() => act(f, f.status === 'new' ? 'dismiss' : 'reopen')}>
                    {f.status === 'new' ? 'Dismiss' : 'Reopen'}
                  </button>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

// ---------- policies ----------

function ProposalActions({ p, onChanged }: { p: Proposal; onChanged: () => void }) {
  const toast = useToast();
  const [busy, setBusy] = useState('');
  const [changes, setChanges] = useState(false);
  const [text, setText] = useState('');
  const answer = async (decision: 'approve' | 'reject' | 'changes') => {
    if (busy) return;
    if (decision === 'reject' && !window.confirm('Reject this policy? Its findings return to the list.')) return;
    setBusy(decision);
    try {
      await answerProposal(p.id, decision, decision === 'changes' ? text.trim() : undefined);
      toast.success(decision === 'approve' ? 'Approved: the pull request is being opened.' : decision === 'changes' ? 'Sent: the policy is redrafted.' : 'Rejected.');
      setChanges(false);
      setText('');
      onChanged();
    } catch (e) {
      toast.error(errorText(e));
    } finally {
      setBusy('');
    }
  };
  const check = async () => {
    setBusy('check');
    try {
      const r = await checkProposal(p.id);
      toast.success(`Pull request: ${r.pr_state || 'checked'}.`);
      onChanged();
    } catch (e) {
      toast.error(errorText(e));
    } finally {
      setBusy('');
    }
  };
  if (p.state === 'awaiting_approval') {
    return (
      <div className="space-y-2">
        <div className="flex flex-wrap gap-2">
          <SubmitButton className="btn btn-primary h-8 px-3 text-sm" loading={busy === 'approve'} loadingLabel="Approving…" onClick={() => answer('approve')}>
            Approve and open PR
          </SubmitButton>
          <button type="button" className="btn btn-secondary h-8 px-3 text-sm" aria-expanded={changes} onClick={() => setChanges((v) => !v)}>
            Needs changes
          </button>
          <SubmitButton className="btn btn-secondary h-8 px-3 text-sm" loading={busy === 'reject'} loadingLabel="Rejecting…" onClick={() => answer('reject')}>
            Reject
          </SubmitButton>
        </div>
        {changes && (
          <form
            className="space-y-2"
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              if (text.trim()) answer('changes');
            }}
          >
            <label htmlFor={`changes-${p.id}`} className="block text-sm text-gray-700">
              What should change?
            </label>
            <textarea
              id={`changes-${p.id}`}
              className="w-full rounded border border-gray-300 px-2 py-1.5 text-sm"
              rows={3}
              maxLength={1000}
              value={text}
              onChange={(e) => setText(e.target.value)}
            />
            <SubmitButton type="submit" className="btn btn-primary h-8 px-3 text-sm" loading={busy === 'changes'} loadingLabel="Sending…" disabled={!text.trim()}>
              Send
            </SubmitButton>
          </form>
        )}
      </div>
    );
  }
  if (p.state === 'pr_open') {
    return (
      <SubmitButton className="btn btn-secondary h-8 px-3 text-sm" loading={busy === 'check'} loadingLabel="Checking…" onClick={check}>
        Check pull request now
      </SubmitButton>
    );
  }
  return null;
}

export function PoliciesTab({ page, canWrite, onChanged, productName }: { page: Page<Proposal, ProposalState>; canWrite: boolean; onChanged: () => void; productName: string }) {
  const [open, setOpen] = useState<string | null>(null);
  if (!page.items.length)
    return <p className="text-sm text-gray-500">No policies for {productName} yet. Select findings and press "Draft policy" to propose one.</p>;
  return (
    <ul className="divide-y divide-gray-100">
      {page.items.map((p) => {
        const expanded = open === p.id || p.state === 'awaiting_approval';
        const pr = safePR(p.pr_url);
        return (
          <li key={p.id} className="py-3 space-y-2">
            <div className="flex items-start gap-3 min-w-0">
              <FileText className="w-4 h-4 mt-0.5 text-gray-500 shrink-0" aria-hidden="true" />
              <div className="min-w-0 flex-1 space-y-1">
                <p className="text-sm font-medium text-gray-900 break-words">{p.title || `Policy from ${p.finding_ids.length} finding(s)`}</p>
                <p className="text-xs text-gray-500 flex flex-wrap gap-x-3 gap-y-1">
                  <span className={`badge whitespace-nowrap ${STATE_CLASS[p.state]}`}>{STATE_LABEL[p.state]}</span>
                  <span>{p.product_name}</span>
                  {p.file_path && <code className="break-all">{p.file_path}</code>}
                  <span>started {fmtDate(p.created_at)}</span>
                  {p.in_repo && <span className="text-green-700">in the synced repo</span>}
                  <Link to={jobHref(p.id)} className="text-primary-700 hover:underline">
                    job
                  </Link>
                  {pr && (
                    <a href={pr} target="_blank" rel="noopener noreferrer" className="text-primary-700 hover:underline inline-flex items-center gap-0.5">
                      pull request <ExternalLink className="w-3 h-3" aria-hidden="true" />
                      <span className="sr-only">(opens in a new tab)</span>
                    </a>
                  )}
                  {p.merge_commit && <span className="font-mono">merged {p.merge_commit.slice(0, 7)}</span>}
                </p>
                {p.state === 'covered' && p.covered_by && <p className="text-sm text-gray-700">Already covered by {p.covered_by}.</p>}
                {(p.state === 'failed' || p.state === 'rejected') && p.error && <p className="text-sm text-gray-700 break-words">{p.error}</p>}
              </div>
              {p.state !== 'awaiting_approval' && (p.policy || p.content) && (
                <button
                  type="button"
                  className="text-xs text-gray-600 hover:text-gray-900 underline shrink-0"
                  aria-expanded={expanded}
                  onClick={() => setOpen(expanded ? null : p.id)}
                >
                  {expanded ? 'Hide' : 'Show'} policy
                </button>
              )}
            </div>
            {expanded && (p.content || p.policy) && (
              <div className="space-y-2 sm:pl-7">
                {p.rationale && <p className="text-sm text-gray-700 break-words">Why: {p.rationale}</p>}
                <pre tabIndex={0} aria-label="Policy file" className="max-h-80 overflow-auto rounded bg-gray-900 text-gray-100 text-xs p-3 whitespace-pre-wrap break-words">
                  {p.content || p.policy}
                </pre>
              </div>
            )}
            {canWrite && (
              <div className="sm:pl-7">
                <ProposalActions p={p} onChanged={onChanged} />
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

// ---------- add finding ----------

const CASE_CODE_RE = /^TC-[A-Z0-9]+-[0-9]{3,4}$/;

export function AddFindingDialog({
  products,
  defaultProduct,
  onClose,
  onAdded,
}: {
  products: ProductOption[];
  defaultProduct: number;
  onClose: () => void;
  onAdded: () => void;
}) {
  const toast = useToast();
  const [pid, setPid] = useState(defaultProduct);
  const [title, setTitle] = useState('');
  const [detail, setDetail] = useState('');
  const [caseCode, setCaseCode] = useState('');
  const [jobId, setJobId] = useState('');
  const [runRef, setRunRef] = useState('');
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const problem = !pid
    ? 'Pick a product.'
    : !title.trim()
      ? 'Give the finding a title.'
      : !detail.trim()
        ? 'Describe what was learned.'
        : caseCode.trim() && !CASE_CODE_RE.test(caseCode.trim())
          ? 'The case code must look like TC-VSYS-008.'
          : '';
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (problem || saving) {
      setErr(problem);
      return;
    }
    setSaving(true);
    setErr('');
    try {
      const r = await addFinding({ product_id: pid, title: title.trim(), detail: detail.trim(), case_code: caseCode.trim() || undefined, job_id: jobId.trim() || undefined, run_ref: runRef.trim() || undefined });
      toast.success(r.merged ? 'This lesson was already known: it was added as more evidence.' : 'Finding added.');
      onAdded();
    } catch (e2) {
      setErr(errorText(e2));
    } finally {
      setSaving(false);
    }
  };
  const field = 'w-full rounded border border-gray-300 px-2 h-9 text-sm';
  return (
    <Modal title="Add a finding" onClose={() => !saving && onClose()}>
      <form className="space-y-3" onSubmit={submit} noValidate>
        <label className="block text-sm text-gray-700">
          Product
          <select className={`${field} bg-white mt-1`} value={pid} onChange={(e) => setPid(Number(e.target.value))}>
            {products.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-sm text-gray-700">
          Title
          <input className={`${field} mt-1`} maxLength={200} value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Path traversal cases need a raw HTTP request" />
        </label>
        <label className="block text-sm text-gray-700">
          What was learned
          <textarea
            className="w-full rounded border border-gray-300 px-2 py-1.5 text-sm mt-1"
            rows={5}
            maxLength={2000}
            value={detail}
            onChange={(e) => setDetail(e.target.value)}
            placeholder="What happened, why, and what a spec should do instead."
          />
        </label>
        <div className="grid gap-3 sm:grid-cols-3">
          <label className="block text-sm text-gray-700">
            Case code
            <input className={`${field} mt-1 font-mono`} maxLength={64} value={caseCode} onChange={(e) => setCaseCode(e.target.value.toUpperCase())} placeholder="optional" />
          </label>
          <label className="block text-sm text-gray-700">
            Job id
            <input className={`${field} mt-1 font-mono`} maxLength={191} value={jobId} onChange={(e) => setJobId(e.target.value)} placeholder="optional" />
          </label>
          <label className="block text-sm text-gray-700">
            Run reference
            <input className={`${field} mt-1`} maxLength={120} value={runRef} onChange={(e) => setRunRef(e.target.value)} placeholder="optional" />
          </label>
        </div>
        {err && (
          <p role="alert" className="text-sm text-red-700 break-words">
            {err}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button type="button" className="btn btn-secondary h-9 px-3 text-sm" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <SubmitButton type="submit" className="btn btn-primary h-9 px-3 text-sm" loading={saving} loadingLabel="Adding…">
            Add finding
          </SubmitButton>
        </div>
      </form>
    </Modal>
  );
}
