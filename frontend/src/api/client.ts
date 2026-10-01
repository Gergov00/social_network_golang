import { tokenStore } from './tokenStore';
import type { TokenResponse } from './types';
import { queueAuth } from './authQueue';

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

type Init = RequestInit & { skipAuth?: boolean };

// Колбэк «сессия потеряна»: его ставит AuthContext, чтобы перевести
// приложение в анонимное состояние. Модуль api/ намеренно не знает про React.
let onAuthLost: () => void = () => {};

export function setOnAuthLost(cb: () => void): void {
  onAuthLost = cb;
}

// Общий промис обновления. Смысл всей конструкции: auth-service делает
// reuse-detection, и ВТОРОЙ одновременный refresh предъявит уже
// использованный токен → auth сочтёт это кражей и отзовёт всю семью
// (RevokeByFamilyID), разлогинив пользователя везде. Поэтому все, кто
// поймал 401, ждут один и тот же промис.
let refreshing: Promise<string> | null = null;
let refreshingEpoch = -1;

export function assertSession(epoch: number): void {
  if (epoch !== tokenStore.epoch()) throw new DOMException('Session changed', 'AbortError');
}

async function refreshToken(): Promise<string> {
  const res = await fetch('/auth/refresh', {
    method: 'POST',
    credentials: 'include',
  });
  if (!res.ok) {
    throw await toError(res);
  }
  const data = (await res.json()) as TokenResponse;
  return data.access_token;
}

// Экспортирована намеренно: authApi.refresh() (вызов bootstrap-а сессии на
// старте приложения) обязан идти через ЭТОТ ЖЕ общий промис, а не запускать
// свой fetch — иначе single-flight guard в apiFetch не спасёт, если refresh
// и обычный запрос, поймавший 401, стартуют одновременно (ровно сценарий
// bootstrap-а: восстановление сессии параллельно с загрузкой ленты/`/me`).
export function refreshOnce(): Promise<string> {
  const epoch = tokenStore.epoch();
  if (!refreshing || refreshingEpoch !== epoch) {
    refreshingEpoch = epoch;
    const promise = queueAuth(() => {
      assertSession(epoch);
      return refreshToken();
    })
      .then((token) => {
        assertSession(epoch);
        tokenStore.set(token);
        return token;
      })
      .catch((err) => {
        if (epoch === tokenStore.epoch()) {
          tokenStore.clear();
          onAuthLost();
        }
        throw err;
      })
      .finally(() => {
        if (refreshing === promise) refreshing = null;
      });
    refreshing = promise;
  }
  return refreshing;
}

async function raw(path: string, init: Init, token: string | null): Promise<Response> {
  const headers = new Headers(init.headers);
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (!init.skipAuth && token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  return fetch(path, { ...init, headers, credentials: 'include' });
}

async function toError(res: Response): Promise<ApiError> {
  // Наружу бэкенд отдаёт {"error": "<http.StatusText>"}; деталей БД там нет.
  let message = res.statusText || 'Request failed';
  try {
    const body = (await res.json()) as { error?: string };
    if (body?.error) message = body.error;
  } catch {
    // тело пустое или не JSON — оставляем statusText
  }
  return new ApiError(res.status, message);
}

export async function apiFetch<T>(path: string, init: Init = {}): Promise<T> {
  const epoch = tokenStore.epoch();
  let res = await raw(path, init, tokenStore.get());
  assertSession(epoch);

  // skipAuth защищает от рекурсии: сам /auth/refresh обновляться не должен.
  if (res.status === 401 && !init.skipAuth) {
    const token = await refreshOnce();
    assertSession(epoch);
    res = await raw(path, init, token);
    assertSession(epoch);
    if (res.status === 401) {
      tokenStore.clear();
      onAuthLost();
    }
  }

  if (!res.ok) {
    throw await toError(res);
  }

  // 204 (logout) и пустое тело: .json() на нём бросит SyntaxError.
  if (res.status === 204 || res.headers.get('Content-Length') === '0') {
    return null as T;
  }
  const text = await res.text();
  assertSession(epoch);
  return (text ? JSON.parse(text) : null) as T;
}
