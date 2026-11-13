import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Loader2, Search } from 'lucide-react';
import Modal from './Modal';
import { useAuth } from '../auth/AuthContext';
import { listUsers, roleBadgeClass, type UserSummary } from '../auth/api';

interface Props {
  onClose: () => void;
}

// "Log in as another user" picker for Admin/Service users. Lists GET
// /api/users, filters by username or full name (most seeded users have no
// full name, so username is the main key), hides the caller and disables
// inactive users. Picking a user calls impersonate(), which reloads the app
// on success; failures show the backend's `{error}` text inline.
export default function ImpersonateDialog({ onClose }: Props) {
  const { user, impersonate } = useAuth();
  const [users, setUsers] = useState<UserSummary[] | null>(null);
  const [loadError, setLoadError] = useState('');
  const [query, setQuery] = useState('');
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState('');
  // State updates are async, so a ref blocks a second click in the same tick.
  const inFlight = useRef(false);

  const load = useCallback(async () => {
    setLoadError('');
    setUsers(null);
    try {
      setUsers(await listUsers());
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Could not load users');
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const visible = useMemo(() => {
    if (!users) return [];
    const q = query.trim().toLowerCase();
    return users.filter((u) => {
      if (u.id === user?.id) return false;
      if (!q) return true;
      return u.username.toLowerCase().includes(q) || (u.fullName || '').toLowerCase().includes(q);
    });
  }, [users, query, user?.id]);

  const pick = async (target: UserSummary) => {
    if (inFlight.current || !target.active) return;
    inFlight.current = true;
    setPending(target.username);
    setError('');
    try {
      // On success the page reloads; keep the row busy until it does.
      await impersonate(target.username);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not log in as that user');
      setPending(null);
      inFlight.current = false;
    }
  };

  const close = useCallback(() => {
    if (!inFlight.current) onClose();
  }, [onClose]);

  return (
    <Modal title="Log in as another user" onClose={close}>
      <p className="text-sm text-gray-500 -mt-2 mb-3">
        You act with that user&apos;s full rights for up to one hour. Every write is logged.
      </p>

      <label className="relative block">
        <span className="sr-only">Filter users</span>
        <Search className="w-4 h-4 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2" aria-hidden="true" />
        <input
          type="search"
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Filter by username or name"
          className="w-full h-9 pl-9 pr-3 rounded-lg border border-gray-300 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
        />
      </label>

      {error && (
        <p role="alert" className="mt-3 text-sm text-red-600 break-words">
          {error}
        </p>
      )}

      <div className="mt-3 border border-gray-200 rounded-lg max-h-80 overflow-y-auto">
        {loadError ? (
          <div className="p-4 text-sm text-red-600 flex flex-wrap items-center justify-between gap-2">
            <span className="break-words min-w-0">{loadError}</span>
            <button
              type="button"
              onClick={() => void load()}
              className="px-3 h-8 rounded-lg text-sm font-medium text-gray-700 border border-gray-300 hover:bg-gray-50"
            >
              Retry
            </button>
          </div>
        ) : users === null ? (
          <div className="p-4 text-sm text-gray-500 flex items-center gap-2">
            <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />
            Loading users…
          </div>
        ) : visible.length === 0 ? (
          <p className="p-4 text-sm text-gray-500">
            {query.trim() ? 'No users match that filter.' : 'No other users.'}
          </p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {visible.map((u) => {
              const busy = pending === u.username;
              const disabled = !u.active || pending !== null;
              return (
                <li key={u.id}>
                  <button
                    type="button"
                    onClick={() => void pick(u)}
                    disabled={disabled}
                    aria-busy={busy}
                    title={u.active ? undefined : 'Inactive users cannot be impersonated'}
                    className="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-gray-50 disabled:cursor-not-allowed disabled:hover:bg-transparent"
                  >
                    <span className={`min-w-0 flex-1 ${u.active ? '' : 'opacity-50'}`}>
                      <span className="block text-sm font-medium text-gray-900 truncate">{u.username}</span>
                      {u.fullName && <span className="block text-xs text-gray-500 truncate">{u.fullName}</span>}
                    </span>
                    {!u.active && <span className="text-[10px] font-medium text-gray-500 shrink-0">Inactive</span>}
                    <span
                      className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full ring-1 ring-inset shrink-0 ${roleBadgeClass(
                        u.role,
                      )}`}
                    >
                      {u.role}
                    </span>
                    {busy && <Loader2 className="w-4 h-4 animate-spin text-gray-500 shrink-0" aria-hidden="true" />}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>

      <div className="flex justify-end mt-4">
        <button
          type="button"
          onClick={close}
          disabled={pending !== null}
          className="px-4 h-9 rounded-lg text-sm font-medium text-gray-700 hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Cancel
        </button>
      </div>
    </Modal>
  );
}
