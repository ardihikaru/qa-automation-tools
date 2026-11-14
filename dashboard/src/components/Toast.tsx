import { ReactNode, createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { X } from 'lucide-react';

type Kind = 'error' | 'success';
interface Toast {
  id: number;
  kind: Kind;
  message: string;
}
interface ToastApi {
  error: (message: string) => void;
  success: (message: string) => void;
}

const Ctx = createContext<ToastApi>({ error: () => undefined, success: () => undefined });
export const useToast = () => useContext(Ctx);

const TTL = 7000;

/** Minimal toast stack, bottom right. Errors are announced assertively, successes politely. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const nextId = useRef(1);
  const timers = useRef(new Map<number, number>());

  const dismiss = useCallback((id: number) => {
    window.clearTimeout(timers.current.get(id));
    timers.current.delete(id);
    setItems((cur) => cur.filter((t) => t.id !== id));
  }, []);

  const push = useCallback(
    (kind: Kind, message: string) => {
      const id = nextId.current++;
      setItems((cur) => [...cur.slice(-3), { id, kind, message }]);
      timers.current.set(id, window.setTimeout(() => dismiss(id), TTL));
    },
    [dismiss],
  );

  useEffect(() => {
    const t = timers.current;
    return () => t.forEach((h) => window.clearTimeout(h));
  }, []);

  const value = useMemo<ToastApi>(() => ({ error: (m) => push('error', m), success: (m) => push('success', m) }), [push]);

  return (
    <Ctx.Provider value={value}>
      {children}
      <div className="fixed bottom-4 right-4 left-4 sm:left-auto sm:w-96 z-[60] flex flex-col gap-2 pointer-events-none">
        {items.map((t) => (
          <div
            key={t.id}
            role={t.kind === 'error' ? 'alert' : 'status'}
            className={`pointer-events-auto toast-enter flex items-start gap-2 rounded-lg border px-3 py-2.5 text-sm shadow-lg break-words min-w-0 ${
              t.kind === 'error' ? 'border-red-200 bg-red-50 text-red-800' : 'border-green-200 bg-green-50 text-green-800'
            }`}
          >
            <span className="flex-1 min-w-0">{t.message}</span>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              aria-label="Dismiss"
              className="shrink-0 rounded p-0.5 opacity-70 hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
            >
              <X className="w-4 h-4" aria-hidden="true" />
            </button>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}
