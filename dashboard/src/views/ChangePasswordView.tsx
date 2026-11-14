import { FormEvent, useMemo, useState } from 'react';
import { Loader2, KeyRound } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { changePassword } from '../auth/api';

// Hick + Miller — three fields, one button. Reached only when the backend
// said mustChangePassword=true, so the user lands here without a choice.
export default function ChangePasswordView() {
  const { user, markPasswordChanged } = useAuth();
  const navigate = useNavigate();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirmNext, setConfirmNext] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const hint = useMemo(() => clientValidate(user?.username || '', current, next, confirmNext), [
    user?.username,
    current,
    next,
    confirmNext,
  ]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (hint) return;
    setSubmitting(true);
    setError(null);
    try {
      await changePassword(current, next);
      markPasswordChanged();
      navigate('/products', { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not change password');
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-sm bg-white shadow-lg rounded-xl p-8">
        <div className="flex flex-col items-center mb-6">
          <div className="w-12 h-12 rounded-full bg-primary-600 text-white flex items-center justify-center mb-3">
            <KeyRound className="w-6 h-6" />
          </div>
          <h1 className="text-xl font-semibold text-gray-900">Set a new password</h1>
          <p className="text-sm text-gray-500 mt-1 text-center">
            You&apos;re using your username as a temporary password. Please choose a new one.
          </p>
        </div>

        <form onSubmit={handleSubmit} noValidate>
          <label className="block text-sm font-medium text-gray-700 mb-1" htmlFor="cp-current">
            Current password
          </label>
          <input
            id="cp-current"
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            className="w-full px-3 py-2 mb-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />

          <label className="block text-sm font-medium text-gray-700 mb-1" htmlFor="cp-new">
            New password
          </label>
          <input
            id="cp-new"
            type="password"
            autoComplete="new-password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            className="w-full px-3 py-2 mb-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />

          <label className="block text-sm font-medium text-gray-700 mb-1" htmlFor="cp-confirm">
            Confirm new password
          </label>
          <input
            id="cp-confirm"
            type="password"
            autoComplete="new-password"
            value={confirmNext}
            onChange={(e) => setConfirmNext(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />

          <p className="mt-2 text-xs text-gray-500">
            At least 8 characters and different from your username.
          </p>

          {(hint || error) && (
            <div className="mt-3 text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">
              {error || hint}
            </div>
          )}

          <button
            type="submit"
            disabled={Boolean(hint) || submitting}
            className="w-full mt-5 h-11 inline-flex items-center justify-center gap-2 rounded-lg bg-primary-600 text-white font-medium hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {submitting ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                Saving…
              </>
            ) : (
              'Save and continue'
            )}
          </button>
        </form>
      </div>
    </div>
  );
}

function clientValidate(username: string, current: string, next: string, confirm: string): string | null {
  if (!current) return 'Enter your current password';
  if (next.length < 8) return 'New password must be at least 8 characters';
  if (next.toLowerCase() === username.toLowerCase()) return 'New password cannot be your username';
  if (next === current) return 'New password must differ from the current one';
  if (next !== confirm) return 'New passwords do not match';
  return null;
}
