// Archive / unarchive one conversation, with a confirmation for archive.
import { useState } from 'react';
import { Archive, ArchiveRestore } from 'lucide-react';
import Modal from '../../components/Modal';
import SubmitButton from '../../components/SubmitButton';
import { useToast } from '../../components/Toast';
import { errorText } from '../knowledge/api';
import { Conversation, archiveConversation, unarchiveConversation } from './api';

export default function ArchiveButton({ conversation, onDone, compact }: { conversation: Conversation; onDone: (c: Conversation) => void; compact?: boolean }) {
  const toast = useToast();
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const archived = !!conversation.archived_at;
  const size = compact ? 'h-8 px-2.5 text-xs' : 'h-9 px-3 text-sm';

  const run = async () => {
    setBusy(true);
    setErr(null);
    try {
      const c = archived ? await unarchiveConversation(conversation.id) : await archiveConversation(conversation.id);
      toast.success(archived ? 'Conversation restored.' : 'Conversation archived.');
      setConfirm(false);
      onDone(c);
    } catch (e) {
      const m = errorText(e);
      setErr(m);
      if (archived) toast.error(`Could not unarchive: ${m}`);
    } finally {
      setBusy(false);
    }
  };

  if (archived) {
    return (
      <SubmitButton loading={busy} loadingLabel="Restoring…" onClick={run} className={`btn btn-secondary inline-flex items-center gap-1.5 ${size}`}>
        <ArchiveRestore className="w-4 h-4" aria-hidden="true" /> Unarchive
      </SubmitButton>
    );
  }
  return (
    <>
      <button type="button" onClick={() => setConfirm(true)} className={`btn btn-secondary inline-flex items-center gap-1.5 ${size}`}>
        <Archive className="w-4 h-4" aria-hidden="true" /> Archive
      </button>
      {confirm && (
        <Modal title="Archive this conversation?" onClose={() => !busy && setConfirm(false)}>
          <p className="text-sm text-gray-700">
            <span className="font-medium">{conversation.title || conversation.ref}</span> ({conversation.message_count} messages) disappears from the job page and the lists. You can
            unarchive it from the Archive page until an Admin prunes the archived conversations.
          </p>
          {err && (
            <p role="alert" className="mt-3 text-sm text-red-700 break-words">
              {err}
            </p>
          )}
          <div className="mt-5 flex justify-end gap-2">
            <button type="button" className="btn btn-secondary h-9 px-3 text-sm" disabled={busy} onClick={() => setConfirm(false)}>
              Cancel
            </button>
            <SubmitButton loading={busy} loadingLabel="Archiving…" onClick={run} className="btn btn-primary h-9 px-3 text-sm">
              Archive
            </SubmitButton>
          </div>
        </Modal>
      )}
    </>
  );
}
