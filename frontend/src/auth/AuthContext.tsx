import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { authApi } from '../api/auth';
import { ApiError, setOnAuthLost } from '../api/client';
import { tokenStore } from '../api/tokenStore';
import { usersApi } from '../api/users';
import type { UserDTO } from '../api/types';

type Status = 'loading' | 'authed' | 'anon';

type AuthValue = {
  status: Status;
  user: UserDTO | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
};

const AuthContext = createContext<AuthValue | null>(null);

// Профиль заводится асинхронно (auth → NATS user.registered → user-service),
// поэтому сразу после регистрации GET /me штатно отдаёт 404. Ждём суммарно
// ~4.5 с нарастающими паузами; если не дождались — пускаем в приложение,
// профиль подтянется позже.
const RETRY_DELAYS = [300, 600, 900, 1200, 1500];

async function fetchMeWithRetry(): Promise<UserDTO | null> {
  for (let attempt = 0; attempt <= RETRY_DELAYS.length; attempt += 1) {
    try {
      return await usersApi.getMe();
    } catch (err) {
      const isMissing = err instanceof ApiError && err.status === 404;
      if (!isMissing || attempt === RETRY_DELAYS.length) {
        if (!isMissing) throw err;
        return null;
      }
      await new Promise((resolve) => setTimeout(resolve, RETRY_DELAYS[attempt]));
    }
  }
  return null;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status>('loading');
  const [user, setUser] = useState<UserDTO | null>(null);

  // Сессию может потерять любой запрос (провалившийся refresh) — тогда
  // приложение обязано перейти в анонимное состояние целиком.
  useEffect(() => {
    setOnAuthLost(() => {
      setUser(null);
      setStatus('anon');
    });
    return () => setOnAuthLost(() => {});
  }, []);

  // Старт: пробуем поднять сессию по httpOnly-cookie.
  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        await authApi.refresh();
        const me = await usersApi.getMe();
        if (!cancelled) {
          setUser(me);
          setStatus('authed');
        }
      } catch {
        if (!cancelled) {
          tokenStore.clear();
          setStatus('anon');
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    await authApi.login(email, password);
    setUser(await usersApi.getMe());
    setStatus('authed');
  }, []);

  const register = useCallback(async (email: string, password: string) => {
    await authApi.register(email, password);
    setUser(await fetchMeWithRetry());
    setStatus('authed');
  }, []);

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } finally {
      tokenStore.clear();
      setUser(null);
      setStatus('anon');
    }
  }, []);

  const refreshUser = useCallback(async () => {
    setUser(await usersApi.getMe());
  }, []);

  const value = useMemo(
    () => ({ status, user, login, register, logout, refreshUser }),
    [status, user, login, register, logout, refreshUser],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth вызван вне AuthProvider');
  return ctx;
}
