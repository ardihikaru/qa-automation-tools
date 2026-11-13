import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, SyncRow, getSync } from './api';

const POLL_MS = 1500;
const MAX_MS = 10 * 60 * 1000;

const settled = (s: SyncRow) => s.status !== 'running' && s.ai_status !== 'pending';

/**
 * Follows one sync: GET syncs/{sid} every 1.5 s while it runs or its AI summary is pending.
 * Stops when it settles, after 10 minutes, on a 404, on `stop()` or on unmount.
 * `onChange(sync, statusChanged)` fires on every poll; `statusChanged` is true when `status` moved
 * (for example running → succeeded, so the caller can reload counts).
 */
export function useSyncFollower(pid: number, onChange: (s: SyncRow, statusChanged: boolean) => void) {
  const [sync, setSync] = useState<SyncRow | null>(null);
  const [following, setFollowing] = useState(false);
  const [timedOut, setTimedOut] = useState(false);
  const [sid, setSid] = useState<number | null>(null);
  const gen = useRef(0);
  const timer = useRef<number | undefined>(undefined);
  const cb = useRef(onChange);
  cb.current = onChange;

  const stop = useCallback(() => {
    gen.current++;
    window.clearTimeout(timer.current);
    setFollowing(false);
  }, []);

  const follow = useCallback(
    (id: number) => {
      const g = ++gen.current;
      setSid(id);
      window.clearTimeout(timer.current);
      const deadline = Date.now() + MAX_MS;
      let lastStatus: string | undefined;
      setFollowing(true);
      setTimedOut(false);
      const tick = async () => {
        if (g !== gen.current) return;
        try {
          const s = await getSync(pid, id);
          if (g !== gen.current) return;
          setSync(s);
          const changed = lastStatus !== undefined && lastStatus !== s.status;
          lastStatus = s.status;
          cb.current(s, changed);
          if (settled(s)) {
            setFollowing(false);
            return;
          }
        } catch (e) {
          if (g !== gen.current) return;
          if (e instanceof ApiError && (e.status === 404 || e.status === 401 || e.status === 403)) {
            setFollowing(false);
            return;
          }
          // Transient (network, 5xx): keep polling until the deadline.
        }
        if (Date.now() > deadline) {
          setFollowing(false);
          setTimedOut(true);
          return;
        }
        timer.current = window.setTimeout(tick, POLL_MS);
      };
      void tick();
    },
    [pid],
  );

  useEffect(
    () => () => {
      gen.current++;
      window.clearTimeout(timer.current);
    },
    [],
  );

  /** True until the first poll of the followed sync has answered. */
  const waiting = following && sync?.id !== sid;
  return { sync, sid, following, waiting, timedOut, follow, stop };
}
