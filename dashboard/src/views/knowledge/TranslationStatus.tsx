import { useEffect, useState } from 'react';
import { Languages } from 'lucide-react';
import { TranslationStatus as Status, getTranslationStatus, isAbort } from './api';

const POLL_MS = 10_000;

/**
 * Compact chip with the progress of the background translation of the knowledge base.
 * Admin-only (the caller decides); renders nothing when the endpoint is missing or fails.
 */
export default function TranslationStatusChip({ pid, refreshKey = 0 }: { pid: number; refreshKey?: number }) {
  const [st, setSt] = useState<Status | null>(null);

  useEffect(() => {
    const ac = new AbortController();
    let timer = 0;
    const tick = () => {
      getTranslationStatus(pid, ac.signal)
        .then((s) => {
          setSt(s);
          if (s.state === 'queued' || s.state === 'running') timer = window.setTimeout(tick, POLL_MS);
        })
        .catch((e) => {
          if (!isAbort(e)) setSt(null);
        });
    };
    tick();
    return () => {
      ac.abort();
      window.clearTimeout(timer);
    };
  }, [pid, refreshKey]);

  if (!st) return null;
  const running = st.state === 'queued' || st.state === 'running';
  const tone = st.failed > 0 ? 'bg-red-50 text-red-800' : running || st.stale > 0 ? 'bg-amber-50 text-amber-800' : 'bg-green-50 text-green-800';
  const label = running ? `Translating ${st.done}/${st.total}` : st.failed > 0 ? `${st.failed} failed` : st.stale > 0 ? `${st.stale} outdated` : 'Translated';
  return (
    <span
      className={`badge ${tone} gap-1`}
      role="status"
      title={`Translation status: ${st.state}. ${st.done} of ${st.total} fields translated, ${st.stale} outdated, ${st.failed} failed.`}
    >
      <Languages className="w-3.5 h-3.5" aria-hidden="true" />
      {label}
    </span>
  );
}
