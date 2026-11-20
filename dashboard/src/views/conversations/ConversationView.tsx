// /conversations/:id: one stored AI conversation (docs/designs/AI_CONVERSATIONS.md section 6).
import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useAuth } from '../../auth/AuthContext';
import ConversationThread from '../../components/ConversationThread';
import { usePageHeader } from '../../components/pageHeader';
import { ApiError, absoluteTime, errorText, isAbort } from '../knowledge/api';
import { ErrorNote, Loading } from '../knowledge/ui';
import ArchiveButton from './ArchiveButton';
import { ConvMessage, Conversation, KIND_LABEL, canReadMessages, formatBytes, getConversation, ownerHref } from './api';

export default function ConversationView() {
  const { id = '' } = useParams();
  const cid = Number(id);
  const { user } = useAuth();
  const [conv, setConv] = useState<Conversation | null>(null);
  const [messages, setMessages] = useState<ConvMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  const allowed = canReadMessages(user);

  usePageHeader({
    title: conv ? `Conversation · ${conv.title || conv.ref}` : 'Conversation',
    crumbs: [{ label: conv?.archived_at ? 'Archive' : 'Run history', to: conv?.archived_at ? '/archive' : '/history?tab=conversations' }],
  });

  useEffect(() => {
    if (!allowed || !Number.isInteger(cid) || cid <= 0) return;
    const ac = new AbortController();
    setError(null);
    getConversation(cid, 0, ac.signal)
      .then((r) => {
        setConv(r.conversation);
        setMessages(r.messages);
      })
      .catch((e) => {
        if (isAbort(e)) return;
        setError(e instanceof ApiError && e.status === 404 ? 'This conversation does not exist (it may have been pruned).' : errorText(e, 'Could not load the conversation'));
      });
    return () => ac.abort();
  }, [cid, allowed, retry]);

  if (!allowed) return <ErrorNote>Your role can see the list of conversations but not their messages.</ErrorNote>;
  if (error) return <ErrorNote onRetry={() => setRetry((n) => n + 1)}>{error}</ErrorNote>;
  if (!conv) return <Loading label="Loading the conversation…" />;
  const owner = ownerHref(conv);

  return (
    <div className="space-y-4 min-w-0">
      <header className="card">
        <div className="card-body flex flex-wrap items-center gap-x-5 gap-y-2 text-sm text-gray-700 min-w-0">
          <h1 className="text-lg font-semibold text-gray-900 break-all">{conv.title || conv.ref}</h1>
          <span className="badge bg-gray-100 text-gray-700">{KIND_LABEL[conv.kind] ?? conv.kind}</span>
          {conv.archived_at && <span className="badge bg-amber-100 text-amber-800">archived {absoluteTime(conv.archived_at)}{conv.archived_by_name && ` by ${conv.archived_by_name}`}</span>}
          {conv.product_name && <span>{conv.product_name}</span>}
          {owner ? (
            <Link to={owner} className="font-mono text-primary-700 hover:underline break-all">
              {conv.ref}
            </Link>
          ) : (
            <span className="font-mono break-all">{conv.ref}</span>
          )}
          <span className="tabular-nums">
            {conv.message_count} messages · {formatBytes(conv.bytes)}
          </span>
          {conv.model && <span className="text-gray-500">{conv.model}</span>}
          <span className="text-gray-500">updated {absoluteTime(conv.updated_at)}</span>
          <span className="ml-auto">
            <ArchiveButton conversation={conv} onDone={setConv} />
          </span>
        </div>
      </header>
      <ConversationThread messages={messages} conversationId={conv.id} />
    </div>
  );
}
