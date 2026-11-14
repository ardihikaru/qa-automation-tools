import { FormEvent, useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { Loader2, LogIn } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';

// Login screen. Conscious UX choices:
//  • Hick + Miller — exactly 2 fields and 1 button. Nothing else.
//  • Jakob — same shape as the taskmgr login the team already knows.
//  • Doherty — submit disabled until both fields filled; spinner inline on click.
//  • Postel — trim/lowercase username before sending.
//  • Fitts — primary button is full-width and 44 px tall.
//  • Peak-End — single calm error message; no red flash-bang.
export default function LoginView() {
  const { user, login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  if (user) {
    const from = (location.state as { from?: { pathname?: string } } | null)?.from?.pathname;
    return <Navigate to={from || '/products'} replace />;
  }

  const canSubmit = username.trim().length > 0 && password.length > 0 && !submitting;

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    setSubmitting(true);
    setError(null);
    try {
      await login(username.trim().toLowerCase(), password);
      navigate('/products', { replace: true });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Invalid username or password';
      setError(message);
      setSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 px-4">
      <div className="w-full max-w-sm bg-white shadow-lg rounded-xl p-8">
        <div className="flex flex-col items-center mb-6">
          <div className="w-12 h-12 rounded-full bg-primary-600 text-white flex items-center justify-center mb-3">
            <LogIn className="w-6 h-6" />
          </div>
          <h1 className="text-xl font-semibold text-gray-900">QA Automation</h1>
          <p className="text-sm text-gray-500 mt-1">Sign in with your team account</p>
        </div>

        <form onSubmit={handleSubmit} noValidate>
          <label className="block text-sm font-medium text-gray-700 mb-1" htmlFor="login-username">
            Username
          </label>
          <input
            id="login-username"
            type="text"
            autoComplete="username"
            autoFocus
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="w-full px-3 py-2 mb-4 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />

          <label className="block text-sm font-medium text-gray-700 mb-1" htmlFor="login-password">
            Password
          </label>
          <input
            id="login-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full px-3 py-2 mb-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-transparent"
          />

          {error && (
            <div className="mt-3 mb-2 text-sm text-red-700 bg-red-50 border border-red-100 rounded-lg px-3 py-2">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={!canSubmit}
            className="w-full mt-4 h-11 inline-flex items-center justify-center gap-2 rounded-lg bg-primary-600 text-white font-medium hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {submitting ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                Signing in…
              </>
            ) : (
              'Sign in'
            )}
          </button>
        </form>

        <p className="mt-6 text-xs text-gray-400 text-center">
          First time? Use your username as the password — you&apos;ll be asked to set a new one.
        </p>
      </div>
    </div>
  );
}
