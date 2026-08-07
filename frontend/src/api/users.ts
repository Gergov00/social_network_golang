import { apiFetch } from './client';
import type { UpdateMePayload, UserDTO } from './types';

export const usersApi = {
  getMe: () => apiFetch<UserDTO>('/me'),

  // Тело — ТОЛЬКО изменённые поля. Сквозная PATCH-семантика: отсутствующее
  // поле значит «не трогать», пустая строка — «очистить». Отправить всю
  // форму значит затереть незаполненное.
  updateMe: (patch: UpdateMePayload) =>
    apiFetch<UserDTO>('/me', { method: 'PATCH', body: JSON.stringify(patch) }),

  getUserByHandle: (handle: string) =>
    apiFetch<UserDTO>(`/users/${encodeURIComponent(handle)}`),
};
