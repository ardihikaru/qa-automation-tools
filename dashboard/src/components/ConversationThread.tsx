// One stored AI conversation, message by message (docs/designs/AI_CONVERSATIONS.md section 6).
// Replies and reviewer feedback are markdown (safe renderer: React elements only, no raw HTML);
// prompts, refusals and run results are machine-built text shown as text nodes in a <pre>.
import { useState } from 'react';
import { ChevronRight } from 'lucide-react';
import Markdown from '../views/assistants/Markdown';
import type { ConvMessage } from '../views/conversations/api';
import { formatBytes } from '../views/conversations/api';

const LONG = 600;

const LABEL: Record<string, string> = {
  prompt: 'Prompt',
  message: 'Message',
  reply: 'AI reply',
  feedback: 'Reviewer feedback',
  refusal: 'Reply refused',
  run_result: 'Run result',
  rebuilt: 'Note',
  error: 'Call failed',
};

const TONE: Record<string, string> = {
  prompt: 'border-gray-200 bg-gray-50',
  message: 'border-gray-200 bg-white',
  reply: 'border-primary-200 bg-white',
  feedback: 'border-amber-300 bg-amber-50',
  refusal: 'border-red-200 bg-red-50/60',
  run_result: 'border-gray-200 bg-gray-50',
  rebuilt: 'border-dashed border-gray-300 bg-white',
  error: 'border-red-300 bg-red-50',
};

/** Written by a model or a person: rendered as markdown. */
const MARKDOWN_KINDS = new Set(['reply', 'feedback']);

const fmtTime = (at: string) => {
  const d = new Date(at);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString([], { hour12: false });
};

function Pre({ text }: { text: string }) {
  return (
    <pre tabIndex={0} className="max-h-[28rem] overflow-auto whitespace-pre-wrap break-words rounded bg-white/70 p-3 font-mono text-xs text-gray-800 [overflow-wrap:anywhere]">
      {text}
    </pre>
  );
}

function MessageBlock({ m, idPrefix }: { m: ConvMessage; idPrefix: string }) {
  const long = m.content.length > LONG;
  const collapsible = (m.kind === 'prompt' || m.kind === 'run_result' || m.kind === 'refusal' || m.role === 'system') && long;
  const [open, setOpen] = useState(!collapsible);
  const label = m.role === 'system' ? 'System prompt' : (LABEL[m.kind] ?? m.kind);
  const md = MARKDOWN_KINDS.has(m.kind) || m.role === 'assistant';
  const note = m.role === 'note';

  return (
    <li className={`rounded-lg border ${TONE[m.kind] ?? 'border-gray-200 bg-white'} min-w-0`} data-kind={m.kind}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-xs text-gray-600">
        <span className="font-semibold text-gray-800">{label}</span>
        {!note && <span className="font-mono">{m.role}</span>}
        <span className="tabular-nums">#{m.seq}</span>
        <time dateTime={m.created_at}>{fmtTime(m.created_at)}</time>
        {collapsible && (
          <button type="button" aria-expanded={open} onClick={() => setOpen((o) => !o)} className="ml-auto inline-flex items-center gap-1 rounded px-2 py-1 text-primary-700 hover:bg-white">
            <ChevronRight className={`h-3.5 w-3.5 transition-transform ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
            {open ? 'Hide' : `Show (${formatBytes(new TextEncoder().encode(m.content).length)})`}
          </button>
        )}
      </div>
      {open && (
        <div className="border-t border-black/5 px-3 py-3 min-w-0">
          {note ? <p className={`text-sm ${m.kind === 'error' ? 'text-red-800' : 'text-gray-600'} break-words`}>{m.content}</p> : md ? <Markdown source={m.content} idPrefix={idPrefix} showLinkHost empty="(empty)" /> : <Pre text={m.content} />}
        </div>
      )}
    </li>
  );
}

export default function ConversationThread({ messages, conversationId }: { messages: ConvMessage[]; conversationId: number }) {
  if (messages.length === 0) return <p className="text-sm text-gray-500">No messages stored.</p>;
  return (
    <ol className="space-y-3" aria-label="AI conversation">
      {messages.map((m) => (
        <MessageBlock key={m.seq} m={m} idPrefix={`c${conversationId}-${m.seq}-`} />
      ))}
    </ol>
  );
}
