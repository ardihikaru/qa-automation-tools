import { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from './AuthContext';

interface Props {
  children: ReactNode;
}

// ProtectedRoute gates everything behind a valid session, and forces a
// password change for first-time logins. The brief "Loading…" panel keeps
// the perceived response under Doherty's 400 ms threshold while /api/auth/me
// resolves.
export function ProtectedRoute({ children }: Props) {
  const { user, loading, mustChangePassword } = useAuth();
  const location = useLocation();

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center text-gray-500">
        <div className="flex items-center gap-3">
          <span className="inline-block w-3 h-3 rounded-full bg-primary-500 animate-pulse" />
          Loading session…
        </div>
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  // The backend refuses password changes while impersonating (403), and the
  // impersonated identity always has mustChangePassword=false.
  if (user.impersonator && location.pathname === '/change-password') {
    return <Navigate to="/" replace />;
  }

  if (mustChangePassword && location.pathname !== '/change-password') {
    return <Navigate to="/change-password" replace />;
  }

  return <>{children}</>;
}
