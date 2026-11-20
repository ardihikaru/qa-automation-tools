// The job page's "AI conversation" card: polls new messages (since_seq) every 2 s while the job runs.
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import ConversationThread from '../../components/ConversationThread';
import { errorText, isAbort } from '../knowledge/api';
import ArchiveButton from './ArchiveButton';
import { ConvMessage, Conversation, conversationHref, formatBytes, getJobConversation } from './api';

const POLL_MS = 2000;

export default function JobConversationCard({ jobId, terminal }: { jobId: string; terminal: boolean }) {
  const [conv, setConv] = useState<Conversation | null>(null);
  const [messages, setMessages] = useState<ConvMessage[]>([]);
  const [restricted, setRestricted] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);
  const seq = useRef(0);

  useEffect(() => {
    seq.current = 0;
    setMessages([]);
    setConv(null);
    setLoaded(false);
  }, [jobId, reload]);

  useEffect(() => {
    const ac = new AbortController();
    let timer: number | undefined;
    let finalPass = false;
    const tick = async () => {
      try {
        const r = await getJobConversation(jobId, seq.current, ac.signal);
        if (ac.signal.aborted) return;
        setConv(r.conversation);
        setRestricted(!!r.restricted);
        const fresh = r.messages.filter((m) => m.seq > seq.current);
        if (fresh.length) {
          seq.current = Math.max(...fresh.map((m) => m.seq));
          setMessages((cur) => [...cur, ...fresh]);
        }
        setError(null);
        setLoaded(true);
      } catch (e) {
        if (ac.signal.aborted || isAbort(e)) return;
        setError(errorText(e, 'Could not load the conversation'));
      }
      // Poll while the job runs, then once more to pick up the last reply.
      if (!terminal || !finalPass) {
        finalPass = terminal;
        timer = window.setTimeout(tick, POLL_MS);
      }
    };
    tick();
    return () => {
      ac.abort();
      window.clearTimeout(timer);
    };
  }, [jobId, terminal, reload]);

  return (
    <section aria-labelledby="job-conversation" className="card min-w-0">
      <div className="card-header flex flex-wrap items-center gap-2">
        <h2 id="job-conversation" className="font-medium text-gray-900">
          AI conversation
        </h2>
        {conv && (
          <span className="text-xs text-gray-500 tabular-nums">
            {conv.message_count} messages · {formatBytes(conv.bytes)}
            {conv.model && ` · ${conv.model}`}
          </span>
        )}
        {conv && (
          <span className="ml-auto flex items-center gap-2">
            {!conv.archived_at && (
              <Link to={conversationHref(conv.id)} className="text-sm text-primary-700 hover:underline">
                Open
              </Link>
            )}
            {!restricted && <ArchiveButton conversation={conv} onDone={() => setReload((n) => n + 1)} compact />}
          </span>
        )}
      </div>
      <div className="card-body min-w-0">
        {error && (
          <p role="alert" className="mb-2 text-sm text-amber-800">
            {error}
          </p>
        )}
        {!loaded ? (
          <p className="text-sm text-gray-500">Loading the conversation…</p>
        ) : !conv ? (
          <p className="text-sm text-gray-500">{terminal ? 'No AI conversation is stored for this job (it may predate conversation history, or it was pruned).' : 'The conversation appears after the first AI reply.'}</p>
        ) : conv.archived_at ? (
          <p className="text-sm text-gray-600">
            This conversation is archived.{' '}
            <Link to={conversationHref(conv.id)} className="text-primary-700 hover:underline">
              Open it
            </Link>{' '}
            or unarchive it here.
          </p>
        ) : restricted ? (
          <p className="text-sm text-gray-600">Your role cannot read the messages of AI conversations.</p>
        ) : (
          <ConversationThread messages={messages} conversationId={conv.id} />
        )}
      </div>
    </section>
  );
}
