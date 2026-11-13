import { useEffect, useRef, useState } from 'react';
import { LogOut, UserCog } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { IMPERSONATOR_ROLES, roleBadgeClass } from '../auth/api';
import ImpersonateDialog from './ImpersonateDialog';
import SubmitButton from './SubmitButton';

// UserChip is the persistent identity surface in the layout. It satisfies
// Recognition-over-Recall — the team can always see who's signed in and as
// what role without opening a menu.
//
// The logout button uses a themed confirm dialog (not native confirm()) so
// the experience stays consistent with the rest of the dashboard.
//
// Admin/Service users (when not already impersonating) also get a "Log in as
// another user" button that opens ImpersonateDialog. While impersonating, the
// chip shows the impersonated identity and logout signs the real user out.
export default function UserChip() {
  const { user, logout } = useAuth();
  const [confirming, setConfirming] = useState(false);
  const [picking, setPicking] = useState(false);

  if (!user) return null;

  const canImpersonate = IMPERSONATOR_ROLES.includes(user.role) && !user.impersonator;
  const realName = user.impersonator
    ? user.impersonator.fullName || user.impersonator.username
    : null;

  const initials = (user.fullName || user.username)
    .split(/\s+/)
    .filter(Boolean)
    .map((p) => p[0]?.toUpperCase())
    .slice(0, 2)
    .join('');

  return (
    <>
      <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-white border border-gray-200 shadow-sm">
        <span className="w-7 h-7 rounded-full bg-primary-600 text-white text-xs font-semibold flex items-center justify-center">
          {initials || '?'}
        </span>
        <div className="flex flex-col leading-tight">
          <span className="text-sm font-medium text-gray-900 truncate max-w-[12rem]" title={user.fullName || user.username}>
            {user.fullName || user.username}
          </span>
          <span
            className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full ring-1 ring-inset w-fit mt-0.5 ${roleBadgeClass(
              user.role,
            )}`}
          >
            {user.role}
          </span>
        </div>
        {canImpersonate && (
          <button
            type="button"
            onClick={() => setPicking(true)}
            aria-label="Log in as another user"
            title="Log in as another user"
            className="ml-1 p-1.5 rounded-full text-gray-500 hover:text-gray-900 hover:bg-gray-100 transition-colors"
          >
            <UserCog className="w-4 h-4" aria-hidden="true" />
          </button>
        )}
        <button
          type="button"
          onClick={() => setConfirming(true)}
          aria-label="Sign out"
          className={`${canImpersonate ? '' : 'ml-1 '}p-1.5 rounded-full text-gray-500 hover:text-gray-900 hover:bg-gray-100 transition-colors`}
        >
          <LogOut className="w-4 h-4" aria-hidden="true" />
        </button>
      </div>

      {confirming && (
        <ConfirmLogout
          message={realName ? `This signs out ${realName} completely.` : 'You can sign back in any time.'}
          onCancel={() => setConfirming(false)}
          onConfirm={logout}
        />
      )}

      {picking && <ImpersonateDialog onClose={() => setPicking(false)} />}
    </>
  );
}

// The confirm button is guarded against double submission; on failure the
// error is shown inline and the button re-enables.
function ConfirmLogout({
  message,
  onCancel,
  onConfirm,
}: {
  message: string;
  onCancel: () => void;
  onConfirm: () => Promise<void>;
}) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const inFlight = useRef(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && !inFlight.current && onCancel();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel]);

  const confirm = async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError('');
    try {
      // On success the user becomes null and this component unmounts.
      await onConfirm();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not sign out');
      setPending(false);
      inFlight.current = false;
    }
  };

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/40 px-4">
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-logout-title"
        className="modal-enter bg-white rounded-xl shadow-xl w-full max-w-sm p-6"
      >
        <h2 id="confirm-logout-title" className="text-lg font-semibold text-gray-900">
          Sign out?
        </h2>
        <p className="text-sm text-gray-500 mt-2 break-words">{message}</p>
        {error && (
          <p role="alert" className="text-sm text-red-600 mt-2 break-words">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2 mt-5">
          <button
            type="button"
            onClick={onCancel}
            disabled={pending}
            className="px-4 h-9 rounded-lg text-sm font-medium text-gray-700 hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Cancel
          </button>
          <SubmitButton
            onClick={confirm}
            loading={pending}
            className="px-4 h-9 rounded-lg text-sm font-medium bg-primary-600 text-white hover:bg-primary-700"
          >
            Sign out
          </SubmitButton>
        </div>
      </div>
    </div>
  );
}
