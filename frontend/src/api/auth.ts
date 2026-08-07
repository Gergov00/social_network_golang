import { apiFetch, refreshOnce } from './client';
import { tokenStore } from './tokenStore';
import type { TokenResponse } from './types';

// skipAuth на всех: старый access-токен этим ручкам не нужен, а на logout и
// refresh он ещё и вреден — они работают по httpOnly-cookie.
export const authApi = {
  async register(email: string, password: string): Promise<void> {
    const res = await apiFetch<TokenResponse>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
      skipAuth: true,
    });
    tokenStore.set(res.access_token);
  },

  async login(email: string, password: string): Promise<void> {
    const res = await apiFetch<TokenResponse>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
      skipAuth: true,
    });
    tokenStore.set(res.access_token);
  },

  // Идёт через тот же single-flight refreshOnce(), что и 401-обработка в
  // apiFetch — НЕ через apiFetch(..., { skipAuth: true }). Это критично на
  // bootstrap-е: восстановление сессии (вызов authApi.refresh()) идёт
  // параллельно с загрузкой ленты/`/me`, которые тоже могут словить 401.
  // Два независимых POST /auth/refresh в этот момент — это второй refresh,
  // предъявляющий уже использованный токен: auth-service сочтёт это кражей
  // и отзовёт всю семью (RevokeByFamilyID), разлогинив пользователя везде.
  // refreshOnce() сам кладёт токен в tokenStore — здесь второй раз это
  // делать не нужно (иначе двойная запись).
  async refresh(): Promise<void> {
    await refreshOnce();
  },

  async logout(): Promise<void> {
    await apiFetch('/auth/logout', { method: 'POST', skipAuth: true });
    tokenStore.clear();
  },
};
