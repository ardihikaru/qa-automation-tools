import { createContext, ReactNode, useCallback, useContext, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import * as authApi from './api';
import type { AuthUser } from './api';

interface AuthState {
  user: AuthUser | null;
  loading: boolean;
  mustChangePassword: boolean;
}

interface AuthContextValue extends AuthState {
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  markPasswordChanged: () => void;
  // Both resolve only after starting a full reload to '/' (no page state from
  // the previous identity survives). They reject with AuthError on failure so
  // the caller can show it inline; a 401 signs out and goes to /login instead.
  impersonate: (username: string) => Promise<void>;
  stopImpersonation: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({
    user: null,
    loading: true,
    mustChangePassword: false,
  });

  const navigate = useNavigate();

  const refresh = useCallback(async () => {
    try {
      const data = await authApi.me();
      if (!data) {
        setState({ user: null, loading: false, mustChangePassword: false });
        return;
      }
      setState({
        user: data.user,
        loading: false,
        mustChangePassword: data.mustChangePassword,
      });
    } catch {
      // Network/5xx: keep whatever identity we had (null on first load) rather
      // than signing the user out; only a 401 from me() means "no session".
      setState((s) => ({ ...s, loading: false }));
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const login = useCallback(async (username: string, password: string) => {
    const data = await authApi.login(username, password);
    setState({
      user: data.user,
      loading: false,
      mustChangePassword: data.mustChangePassword,
    });
  }, []);

  const logout = useCallback(async () => {
    await authApi.logout();
    setState({ user: null, loading: false, mustChangePassword: false });
  }, []);

  const signedOut = useCallback(() => {
    setState({ user: null, loading: false, mustChangePassword: false });
    navigate('/login', { replace: true });
  }, [navigate]);

  const impersonate = useCallback(
    async (username: string) => {
      try {
        await authApi.impersonate(username);
      } catch (err) {
        if (err instanceof authApi.AuthError && err.status === 401) {
          signedOut();
          return;
        }
        throw err;
      }
      window.location.assign('/');
    },
    [signedOut],
  );

  const stopImpersonation = useCallback(async () => {
    try {
      await authApi.stopImpersonation();
    } catch (err) {
      if (err instanceof authApi.AuthError && err.status === 401) {
        signedOut();
        return;
      }
      throw err;
    }
    window.location.assign('/');
  }, [signedOut]);

  const markPasswordChanged = useCallback(() => {
    setState((s) => ({
      ...s,
      mustChangePassword: false,
      user: s.user ? { ...s.user, mustChangePassword: false } : null,
    }));
  }, []);

  return (
    <AuthContext.Provider value={{
        ...state,
        login,
        logout,
        refresh,
        markPasswordChanged,
        impersonate,
        stopImpersonation,
      }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>');
  return ctx;
}
