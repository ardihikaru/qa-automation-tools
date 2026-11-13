import { useEffect, useRef, useState } from 'react';
import { Undo2 } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { roleBadgeClass } from '../auth/api';
import SubmitButton from './SubmitButton';

// Full-width amber bar shown above the header while the real user (an Admin or
// the Service account) is acting as someone else. It re-reads the session on
// window focus so an expired impersonation, which the backend silently turns
// back into the real user, clears the bar.
export default function ImpersonationBanner() {
  const { user, refresh, stopImpersonation } = useAuth();
  const [returning, setReturning] = useState(false);
  const [error, setError] = useState('');
  const inFlight = useRef(false);
  const impersonating = Boolean(user?.impersonator);

  useEffect(() => {
    if (!impersonating) return;
    const onFocus = () => {
      void refresh();
    };
    window.addEventListener('focus', onFocus);
    return () => window.removeEventListener('focus', onFocus);
  }, [impersonating, refresh]);

  if (!user || !user.impersonator) return null;

  const actingAs = user.fullName || user.username;
  const realName = user.impersonator.fullName || user.impersonator.username;

  const onReturn = async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    setReturning(true);
    setError('');
    try {
      // On success the page reloads, so `returning` stays true until then.
      await stopImpersonation();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not return to your account');
      setReturning(false);
      inFlight.current = false;
    }
  };

  return (
    <div
      role="status"
      className="w-full bg-amber-100 border-b border-amber-300 text-amber-900 px-4 py-2 text-sm"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <p className="min-w-0 flex-1 flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="min-w-0 inline-flex flex-wrap items-center gap-x-1.5 gap-y-1">
            Viewing as <strong className="font-semibold break-all">{actingAs}</strong>
            <span
              className={`text-[10px] font-medium px-1.5 py-0.5 rounded-full ring-1 ring-inset ${roleBadgeClass(
                user.role,
              )}`}
            >
              {user.role}
            </span>
          </span>
          <span className="min-w-0">
            Signed in as <strong className="font-semibold break-all">{realName}</strong>.
          </span>
        </p>
        <SubmitButton
          onClick={onReturn}
          loading={returning}
          loadingLabel="Returning…"
          className="h-8 px-3 rounded-lg bg-amber-600 text-white font-medium hover:bg-amber-700 shrink-0 max-w-full"
        >
          <Undo2 className="w-4 h-4" aria-hidden="true" />
          <span className="truncate">Return to {realName}</span>
        </SubmitButton>
      </div>
      {error && (
        <p role="alert" className="mt-1 text-xs text-red-700">
          {error}
        </p>
      )}
    </div>
  );
}
