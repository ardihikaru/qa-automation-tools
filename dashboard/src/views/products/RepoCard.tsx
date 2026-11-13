import { useState } from 'react';
import { CheckCircle2, Download, ExternalLink, Pencil, Plus, RefreshCw, Trash2, X, XCircle } from 'lucide-react';
import SubmitButton from '../../components/SubmitButton';
import { RepoCheckIcon } from '../../components/icons';
import {
  BranchDraft,
  BranchResult,
  CHECK_LABELS,
  MAX_BRANCHES,
  Repo,
  branchNameProblem,
  branchProblem,
} from './api';

// ---- branch list editor (also used by the add-product and add-repo forms) ----

export function BranchesField({
  value,
  onChange,
  disabled,
  name,
}: {
  value: BranchDraft;
  onChange: (d: BranchDraft) => void;
  disabled?: boolean;
  name: string;
}) {
  const [text, setText] = useState('');
  const [localError, setLocalError] = useState<string | null>(null);

  function add() {
    const b = text.trim();
    if (!b) return;
    const bad = branchNameProblem(b);
    if (bad) return setLocalError(bad);
    if (value.branches.includes(b)) return setLocalError(`Duplicate branch "${b}"`);
    if (value.branches.length >= MAX_BRANCHES) return setLocalError(`At most ${MAX_BRANCHES} branches per repo`);
    setLocalError(null);
    onChange({ branches: [...value.branches, b], default: value.default || b });
    setText('');
  }
  function remove(b: string) {
    const rest = value.branches.filter((x) => x !== b);
    onChange({ branches: rest, default: value.default === b ? rest[0] ?? '' : value.default });
  }

  return (
    <fieldset className="space-y-2" disabled={disabled}>
      <legend className="block text-sm font-medium text-gray-700 mb-1">Branches (one default)</legend>
      {value.branches.length === 0 ? (
        <p className="text-xs text-gray-500">No branches listed.</p>
      ) : (
        <ul className="space-y-1">
          {value.branches.map((b) => (
            <li key={b} className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name={`${name}-default`}
                checked={value.default === b}
                onChange={() => onChange({ ...value, default: b })}
                aria-label={`Make ${b} the default branch`}
              />
              <code className="break-all flex-1 min-w-0">{b}</code>
              {value.default === b && <span className="badge badge-success">Default</span>}
              <button type="button" className="p-1 text-gray-500 hover:text-red-600 disabled:opacity-50" aria-label={`Remove branch ${b}`} onClick={() => remove(b)}>
                <X className="w-4 h-4" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2">
        <input
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              add();
            }
          }}
          placeholder="release/1.0"
          aria-label="New branch name"
          className="flex-1 min-w-0 px-3 py-1.5 text-sm border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
        />
        <button type="button" className="btn btn-secondary text-xs px-2 py-1 inline-flex items-center gap-1 disabled:opacity-50" onClick={add} disabled={!text.trim()}>
          <Plus className="w-4 h-4" /> Add
        </button>
      </div>
      {localError && <p role="alert" className="text-xs text-red-700">{localError}</p>}
    </fieldset>
  );
}

// ---- sync results ----

const RESULT_CHIP: Record<BranchResult['status'], string> = {
  ok: 'badge badge-success',
  kept_dirty: 'badge bg-amber-100 text-amber-800',
  missing_remote: 'badge badge-error',
  error: 'badge badge-error',
};

export function SyncResults({ results, syncedAt }: { results: BranchResult[]; syncedAt?: string }) {
  if (results.length === 0) return null;
  return (
    <div className="mt-3">
      <div className="text-xs font-medium text-gray-600 mb-1">
        Last sync{syncedAt ? ` (${new Date(syncedAt).toLocaleString()})` : ''}
      </div>
      <ul className="space-y-1">
        {results.map((r) => (
          <li key={r.name} className="flex flex-wrap items-start gap-2 text-sm">
            <span className={RESULT_CHIP[r.status] ?? 'badge bg-gray-100 text-gray-700'}>{r.status}</span>
            <code className="break-all">{r.name}</code>
            {r.message && <span className="text-gray-500 break-words min-w-0">{r.message}</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

// ---- repo card ----

export type RepoAction = 'verify' | 'clone' | 'checkout' | 'branches' | 'main' | 'central' | 'delete' | 'edit';

interface Props {
  repo: Repo;
  allowWork: boolean;
  draft: BranchDraft;
  onDraft: (d: BranchDraft) => void;
  results: BranchResult[];
  syncedAt?: string;
  poolEnabled: boolean;
  canManage: boolean;
  canVerify: boolean;
  /** Key of the in-flight action, e.g. "7:verify"; any in-flight action disables the rest. */
  busyKey: string | null;
  onAction: (repo: Repo, action: RepoAction) => void;
}

export default function RepoCard({ repo, allowWork, draft, onDraft, results, syncedAt, poolEnabled, canManage, canVerify, busyKey, onAction }: Props) {
  const [showChecks, setShowChecks] = useState(false);
  const anyBusy = busyKey !== null;
  const loading = (a: RepoAction) => busyKey === `${repo.id}:${a}`;
  const legacyEmpty = repo.branches.length === 0;
  const dirty = draft.default !== repo.default_branch || draft.branches.join('\n') !== repo.branches.join('\n');
  const problem = branchProblem(draft, legacyEmpty);
  const report = repo.verify_report;
  const verified = repo.last_verified_at !== null;
  const ok = report?.ok ?? repo.playwright_accessible;
  const manageTitle = canManage ? undefined : 'Your role cannot manage products';

  return (
    <section className="border border-gray-200 rounded-lg p-4" aria-label={`Repo ${repo.name}`}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="font-semibold text-gray-900 break-all">{repo.name}</h4>
            {repo.is_main && <span className="badge badge-success">Main</span>}
            {repo.is_central && <span className="badge bg-indigo-100 text-indigo-800">Central</span>}
            {!repo.is_main && !allowWork && <span className="badge bg-gray-100 text-gray-700" title="Read-only context repo">Read only</span>}
            {!verified ? (
              <span className="badge bg-gray-100 text-gray-700">Unverified</span>
            ) : ok ? (
              <span className="badge badge-success">Verified</span>
            ) : (
              <span className="badge badge-error">Checks failed</span>
            )}
          </div>
          <a href={repo.repo_url} target="_blank" rel="noreferrer" className="text-xs text-primary-600 hover:underline inline-flex items-center gap-1 break-all">
            {repo.repo_url.replace('https://github.com/', '')} <ExternalLink className="w-3 h-3 shrink-0" />
          </a>
          <div className="text-xs text-gray-500 break-all">
            <code>{repo.root_dir}</code>
            {verified && <> · verified {new Date(repo.last_verified_at as string).toLocaleString()}</>}
          </div>
        </div>
        <div className="flex flex-wrap gap-1 justify-end">
          <SubmitButton
            className="btn btn-secondary text-xs px-2 py-1"
            loading={loading('verify')}
            loadingLabel="Verifying…"
            disabled={anyBusy || !poolEnabled || !canVerify}
            onClick={() => onAction(repo, 'verify')}
            title="Check the checkout, origin and (main repo) Playwright and staging URL"
          >
            <RepoCheckIcon className="w-4 h-4" /> Verify
          </SubmitButton>
          <SubmitButton
            className="btn btn-secondary text-xs px-2 py-1"
            loading={loading('clone')}
            loadingLabel="Cloning…"
            disabled={anyBusy || !poolEnabled || !canManage}
            title={manageTitle ?? 'gh repo clone into the pool, then sync branches'}
            onClick={() => onAction(repo, 'clone')}
          >
            <Download className="w-4 h-4" /> Clone
          </SubmitButton>
          <SubmitButton
            className="btn btn-secondary text-xs px-2 py-1"
            loading={loading('checkout')}
            loadingLabel="Syncing…"
            disabled={anyBusy || !poolEnabled || !canManage || dirty}
            title={dirty ? 'Save the branch list first' : manageTitle ?? 'Fetch and check out every listed branch'}
            onClick={() => onAction(repo, 'checkout')}
          >
            <RefreshCw className="w-4 h-4" /> Sync branches
          </SubmitButton>
          <button
            className="btn btn-secondary text-xs px-2 py-1 disabled:opacity-50 disabled:cursor-not-allowed"
            disabled={anyBusy || !canManage}
            title={manageTitle}
            aria-label={`Edit ${repo.name}`}
            onClick={() => onAction(repo, 'edit')}
          >
            <Pencil className="w-4 h-4" />
          </button>
          <SubmitButton
            className="btn btn-secondary text-xs px-2 py-1 text-red-600"
            loading={loading('delete')}
            disabled={anyBusy || !canManage || repo.is_main}
            title={repo.is_main ? 'Promote another repo to main first' : manageTitle}
            aria-label={`Remove ${repo.name}`}
            onClick={() => onAction(repo, 'delete')}
          >
            <Trash2 className="w-4 h-4" />
          </SubmitButton>
        </div>
      </div>

      <div className="flex flex-wrap gap-2 mt-3">
        {!repo.is_main && (
          <SubmitButton
            className="btn btn-secondary text-xs px-2 py-1"
            loading={loading('main')}
            loadingLabel="Promoting…"
            disabled={anyBusy || !canManage}
            title={manageTitle ?? 'Make this the main repo; the current main becomes a sibling'}
            onClick={() => onAction(repo, 'main')}
          >
            Make main
          </SubmitButton>
        )}
        <SubmitButton
          className="btn btn-secondary text-xs px-2 py-1"
          loading={loading('central')}
          loadingLabel="Saving…"
          disabled={anyBusy || !canManage}
          title={manageTitle ?? (repo.is_central ? 'Clear the central flag' : 'Mark as central repo (moves the flag from another repo)')}
          onClick={() => onAction(repo, 'central')}
        >
          {repo.is_central ? 'Unset central' : 'Make central'}
        </SubmitButton>
        {report && (
          <button type="button" className="text-xs text-primary-600 hover:underline" aria-expanded={showChecks} onClick={() => setShowChecks((s) => !s)}>
            {showChecks ? 'Hide checks' : 'Show checks'}
          </button>
        )}
      </div>

      {showChecks && report && (
        <ul className="space-y-1 mt-3">
          {report.checks.map((c) => (
            <li key={c.name} className="flex items-start gap-2 text-sm">
              {c.ok ? <CheckCircle2 className="w-4 h-4 text-green-600 mt-0.5 shrink-0" /> : <XCircle className="w-4 h-4 text-red-600 mt-0.5 shrink-0" />}
              <span className="font-medium">{CHECK_LABELS[c.name] || c.name}</span>
              {c.detail && <span className="text-gray-500 break-all">{c.detail}</span>}
            </li>
          ))}
        </ul>
      )}

      <div className="mt-4">
        <BranchesField value={draft} onChange={onDraft} disabled={anyBusy || !canManage} name={`repo-${repo.id}`} />
        {legacyEmpty && !dirty && <p className="text-xs text-gray-500 mt-1">Legacy repo without a branch list: the clone keeps its current branch until you add branches.</p>}
        {dirty && problem && <p role="alert" className="text-xs text-red-700 mt-1">{problem}</p>}
        <div className="flex gap-2 mt-2">
          <SubmitButton
            className="btn btn-primary text-xs px-2 py-1"
            loading={loading('branches')}
            loadingLabel="Saving…"
            disabled={anyBusy || !canManage || !dirty || problem !== null}
            title={manageTitle}
            onClick={() => onAction(repo, 'branches')}
          >
            Save branches
          </SubmitButton>
          {dirty && (
            <button
              type="button"
              className="btn btn-secondary text-xs px-2 py-1 disabled:opacity-50"
              disabled={anyBusy}
              onClick={() => onDraft({ branches: repo.branches, default: repo.default_branch })}
            >
              Discard changes
            </button>
          )}
        </div>
      </div>

      <SyncResults results={results} syncedAt={syncedAt} />
    </section>
  );
}
