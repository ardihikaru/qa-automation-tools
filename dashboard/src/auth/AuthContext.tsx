import { createContext, ReactNode, useCallback, useContext, useEffect, useState } from 'react';
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
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({
    user: null,
    loading: true,
    mustChangePassword: false,
  });

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
      setState({ user: null, loading: false, mustChangePassword: false });
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

  const markPasswordChanged = useCallback(() => {
    setState((s) => ({
      ...s,
      mustChangePassword: false,
      user: s.user ? { ...s.user, mustChangePassword: false } : null,
    }));
  }, []);

  return (
    <AuthContext.Provider value={{ ...state, login, logout, refresh, markPasswordChanged }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>');
  return ctx;
}
