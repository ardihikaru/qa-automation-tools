import { useEffect, useState } from 'react';
import { errorText, isAbort } from './api';

export function useDebounced<T>(value: T, ms = 300): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(t);
  }, [value, ms]);
  return v;
}

/**
 * Loads `fetcher` whenever `key` changes. Older requests are aborted, so a slow response
 * can never overwrite a newer one. The previous data stays visible while the next page loads.
 */
export function useLoad<T>(key: string, fetcher: (signal: AbortSignal) => Promise<T>) {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    const ac = new AbortController();
    setLoading(true);
    setError(null);
    fetcher(ac.signal)
      .then((d) => {
        if (ac.signal.aborted) return;
        setData(d);
        setLoading(false);
      })
      .catch((e) => {
        if (ac.signal.aborted || isAbort(e)) return;
        setError(errorText(e, 'Could not load'));
        setLoading(false);
      });
    return () => ac.abort();
    // `fetcher` is rebuilt every render; `key` captures everything it depends on.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, retry]);

  return { data, setData, loading, error, reload: () => setRetry((n) => n + 1) };
}
