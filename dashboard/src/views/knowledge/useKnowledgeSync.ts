import { useCallback, useEffect, useRef, useState } from 'react';
import { SyncRow, errorText, startSync } from './api';
import { useSyncFollower } from './useSyncFollower';

const live = (s: SyncRow | null | undefined) => !!s && (s.status === 'running' || s.ai_status === 'pending');

/**
 * Start-and-follow for a knowledge sync, shared by the products list rows and the knowledge
 * base top bar. `fromSummary` is the latest sync the caller already knows (from GET summary);
 * a live one is followed automatically. `onSettled` fires when the followed sync changes
 * status or settles, so the caller can reload its summary.
 *
 * `start()` is guarded synchronously: a double click sends one POST. A 409 for a sync that is
 * already running attaches to it (`note` says so).
 */
export function useKnowledgeSync(pid: number, fromSummary: SyncRow | null, onSettled: () => void) {
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const inflight = useRef(false);
  const settledCb = useRef(onSettled);
  settledCb.current = onSettled;

  const follower = useSyncFollower(pid, (s, statusChanged) => {
    if (!live(s)) setNote(null);
    if (statusChanged || !live(s)) settledCb.current();
  });
  const { follow, following, timedOut } = follower;

  // Resume polling when the caller loads on a running sync (or one whose summary is pending).
  const latestId = fromSummary?.id;
  const latestLive = live(fromSummary);
  const followedId = follower.sync?.id;
  useEffect(() => {
    if (latestId && latestLive && !following && !timedOut && followedId !== latestId) follow(latestId);
  }, [latestId, latestLive, following, follow, followedId, timedOut]);

  const latest = follower.sync && (!fromSummary || follower.sync.id >= fromSummary.id) ? follower.sync : fromSummary;
  const running = latest?.status === 'running';
  const busy = starting || running || follower.waiting;

  const start = useCallback(async () => {
    if (inflight.current || running) return;
    inflight.current = true;
    setStarting(true);
    setError(null);
    setNote(null);
    try {
      const r = await startSync(pid);
      if (r.attached) setNote(`Sync #${r.sync_id} was already running; following it.`);
      follow(r.sync_id);
      settledCb.current();
    } catch (e) {
      setError(errorText(e, 'Could not start the sync'));
    } finally {
      inflight.current = false;
      setStarting(false);
    }
  }, [pid, running, follow]);

  return { latest, running, busy, error, note, timedOut, start };
}
