import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { authApi } from '../api/auth';
import { ApiError, assertSession, setOnAuthLost } from '../api/client';
import { tokenStore } from '../api/tokenStore';
import { usersApi } from '../api/users';
import type { UserDTO } from '../api/types';

type Status = 'loading' | 'authed' | 'anon';

type AuthValue = {
  status: Status;
  user: UserDTO | null;
  // Причина, по которой user===null при status==='authed': профиль ещё не
  // создан (404 после исчерпанных ретраев) либо /me моргнул (5xx/сеть).
  // null означает «ошибки нет» (обычно потому что user уже загружен).
  meError: string | null;
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

const PROFILE_PENDING_MESSAGE = 'Профиль ещё создаётся. Попробуйте обновить через минуту.';
const PROFILE_LOAD_FAILED_MESSAGE = 'Не удалось загрузить профиль. Попробуйте ещё раз.';

type MeOutcome = { user: UserDTO | null; error: string | null };

// Единая политика для ВСЕХ вызовов /me (bootstrap, login, register,
// ручной retry): различает три причины отказа.
// - 401 (сессии нет — refresh уже провалился внутри apiFetch) — пробрасываем
//   дальше НЕ трогая state: onAuthLost (см. setOnAuthLost ниже) уже перевёл
//   приложение в 'anon', и эта функция не должна перезаписать его обратно
//   в 'authed'.
// - 404 — профиль ещё не создан асинхронным консьюмером, ретраим с паузами;
//   не дождались — остаёмся authed, но с человеческим объяснением.
// - всё остальное (5xx/504/сетевой обрыв) — бэкенд моргнул: не ретраим
//   бесконечно, остаёмся authed, отдаём текст для кнопки «Повторить».
async function resolveMe(): Promise<MeOutcome> {
  const epoch = tokenStore.epoch();
  for (let attempt = 0; attempt <= RETRY_DELAYS.length; attempt += 1) {
    assertSession(epoch);
    try {
      const user = await usersApi.getMe();
      assertSession(epoch);
      return { user, error: null };
    } catch (err) {
      assertSession(epoch);
      if (err instanceof ApiError && err.status === 401) throw err;

      const isMissing = err instanceof ApiError && err.status === 404;
      if (isMissing && attempt < RETRY_DELAYS.length) {
        await new Promise((resolve) => setTimeout(resolve, RETRY_DELAYS[attempt]));
        continue;
      }
      return { user: null, error: isMissing ? PROFILE_PENDING_MESSAGE : PROFILE_LOAD_FAILED_MESSAGE };
    }
  }
  return { user: null, error: PROFILE_PENDING_MESSAGE };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<Status>('loading');
  const [user, setUser] = useState<UserDTO | null>(null);
  const [meError, setMeError] = useState<string | null>(null);

  // Все границы сессии синхронно очищают клиентские данные. Отмена queries
  // останавливает retry; epoch в API и мутациях блокирует поздние callbacks.
  useEffect(() => {
    const reset = () => {
      void queryClient.cancelQueries();
      queryClient.clear();
      setUser(null);
      setMeError(null);
      setStatus('anon');
    };
    const unsubscribe = tokenStore.subscribe(reset);
    setOnAuthLost(reset);
    return () => { unsubscribe(); setOnAuthLost(() => {}); };
  }, [queryClient]);

  // Старт: пробуем поднять сессию по httpOnly-cookie. Провал именно
  // authApi.refresh() (401/сессии нет) — единственный повод остаться/стать
  // 'anon'; здесь же onAuthLost уже сделал всю работу через single-flight
  // refreshOnce(), tokenStore.clear() ниже — подстраховка на случай, если
  // колбэк почему-то не сработал.
  useEffect(() => {
    let cancelled = false;
    const epoch = tokenStore.epoch();

    (async () => {
      try {
        await authApi.refresh();
      } catch {
        if (!cancelled && epoch === tokenStore.epoch()) {
          tokenStore.clear();
          setStatus('anon');
        }
        return;
      }

      if (cancelled || epoch !== tokenStore.epoch()) return;

      try {
        const { user: me, error } = await resolveMe();
        if (!cancelled && epoch === tokenStore.epoch()) {
          setUser(me);
          setMeError(error);
          setStatus('authed');
        }
      } catch {
        // /me ответил 401 после успешного refresh — по сути повторный провал
        // сессии (гонка); onAuthLost уже перевёл в 'anon', здесь ничего
        // делать не нужно (и НЕЛЬЗЯ перезаписать его обратно в 'authed').
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    tokenStore.clear();
    const epoch = tokenStore.epoch();
    // Токен уже лежит в tokenStore к этому моменту (authApi.login это делает).
    // Дальнейший провал /me (404/5xx) — НЕ повод сказать пользователю
    // «не удалось войти»: логин прошёл успешно.
    await authApi.login(email, password);
    assertSession(epoch);
    try {
      const { user: me, error } = await resolveMe();
      assertSession(epoch);
      setUser(me);
      setMeError(error);
      setStatus('authed');
    } catch {
      // /me ответил 401 сразу после успешного логина — сессия уже потеряна
      // (гонка), onAuthLost перевёл в 'anon'; перезаписывать его на 'authed'
      // нельзя.
    }
  }, []);

  const register = useCallback(async (email: string, password: string) => {
    tokenStore.clear();
    const epoch = tokenStore.epoch();
    await authApi.register(email, password);
    assertSession(epoch);
    try {
      const { user: me, error } = await resolveMe();
      assertSession(epoch);
      setUser(me);
      setMeError(error);
      setStatus('authed');
    } catch {
      // см. комментарий в login выше.
    }
  }, []);

  const logout = useCallback(async () => {
    // authApi.logout завершает локальную сессию до сетевого запроса.
    // Topbar вызывает void logout(): ошибку запроса поглощаем здесь.
    try {
      await authApi.logout();
    } catch (err) {
      console.error('POST /auth/logout не удался (сессия всё равно завершена локально)', err);
    }
  }, []);

  const refreshUser = useCallback(async () => {
    const epoch = tokenStore.epoch();
    try {
      const { user: me, error } = await resolveMe();
      assertSession(epoch);
      setUser(me);
      setMeError(error);
    } catch {
      // 401 — сессия потеряна, onAuthLost уже отработал.
    }
  }, []);

  const value = useMemo(
    () => ({ status, user, meError, login, register, logout, refreshUser }),
    [status, user, meError, login, register, logout, refreshUser],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth вызван вне AuthProvider');
  return ctx;
}
