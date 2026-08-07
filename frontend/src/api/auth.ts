import { apiFetch } from './client';
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

  async refresh(): Promise<void> {
    const res = await apiFetch<TokenResponse>('/auth/refresh', {
      method: 'POST',
      skipAuth: true,
    });
    tokenStore.set(res.access_token);
  },

  async logout(): Promise<void> {
    await apiFetch('/auth/logout', { method: 'POST', skipAuth: true });
    tokenStore.clear();
  },
};
