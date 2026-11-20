// "Prune all archived": preview, type "prune", delete (real Admin only; the server checks too).
import { useEffect, useState } from 'react';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import { useToast } from '../../components/Toast';
import { errorText, isAbort } from '../knowledge/api';
import { inputCls } from '../knowledge/styles';
import { PrunePreview, formatBytes, getPrunePreview, pruneArchived } from './api';

export default function PruneDialog({ onClose, onPruned }: { onClose: () => void; onPruned: () => void }) {
  const toast = useToast();
  const [preview, setPreview] = useState<PrunePreview | null>(null);
  const [typed, setTyped] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    const ac = new AbortController();
    getPrunePreview(ac.signal)
      .then(setPreview)
      .catch((e) => !isAbort(e) && setErr(errorText(e, 'Could not load what would be pruned')));
    return () => ac.abort();
  }, []);

  const ready = !!preview?.archived_before && typed.trim().toLowerCase() === 'prune' && !busy;
  const run = async () => {
    if (!preview?.archived_before) return;
    setBusy(true);
    setErr(null);
    try {
      const r = await pruneArchived(preview.archived_before);
      toast.success(`Pruned ${r.conversations} conversation${r.conversations === 1 ? '' : 's'} (${formatBytes(r.bytes)}).`);
      onPruned();
      onClose();
    } catch (e) {
      setErr(errorText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title="Prune all archived conversations?" onClose={() => !busy && onClose()}>
      {!preview && !err && <p className="text-sm text-gray-600">Counting the archived conversations…</p>}
      {preview && preview.conversations === 0 && <p className="text-sm text-gray-700">Nothing is archived. There is nothing to prune.</p>}
      {preview && preview.conversations > 0 && (
        <div className="space-y-3 text-sm text-gray-700">
          <p>
            This <strong>permanently deletes</strong> {preview.conversations} archived conversation{preview.conversations === 1 ? '' : 's'} with {preview.messages} messages ({formatBytes(preview.bytes)}). It cannot be undone.
          </p>
          <p className="text-gray-600">
            Job records, step times, the run history, knowledge sync summaries and AI cost records are kept. Conversations archived after you opened this dialog are not included.
          </p>
          <label className="block">
            <span className="block mb-1 font-medium">
              Type <span className="font-mono">prune</span> to confirm
            </span>
            <input className={`${inputCls} w-full`} value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" spellCheck={false} aria-describedby={err ? 'prune-err' : undefined} />
          </label>
        </div>
      )}
      {err && (
        <p id="prune-err" role="alert" className="mt-3 text-sm text-red-700 break-words">
          {err}
        </p>
      )}
      <div className="mt-5 flex justify-end gap-2">
        <button type="button" className="btn btn-secondary h-9 px-3 text-sm" disabled={busy} onClick={onClose}>
          Cancel
        </button>
        <SubmitButton loading={busy} loadingLabel="Pruning…" disabled={!ready} onClick={run} className="btn btn-danger h-9 px-3 text-sm">
          Prune permanently
        </SubmitButton>
      </div>
    </Modal>
  );
}
