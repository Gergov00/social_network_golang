import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { http, HttpResponse } from 'msw';
import { expect, test } from 'vitest';
import { server } from '../test/server';
import { createQueryClient } from '../app/providers';
import { AuthProvider, useAuth } from './AuthContext';
import { tokenStore } from '../api/tokenStore';
import { apiFetch } from '../api/client';
import { useLike } from '../features/feed/useLike';
import type { UserDTO } from '../api/types';

const user = (id: string): UserDTO => ({ id, name: id, handle: id, bio: '', city: '', work: '', verified: false, avatarUrl: null, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' });
function deferred() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => { release = resolve; });
  return { promise, release };
}

test('два аккаунта изолированы от старого /me и завершений мутации', async () => {
  const meGate = deferred();
  const likeGate = deferred();
  const feedGate = deferred();
  let delayedMe = false;
  let startedMe = false;
  let startedLike = false;
  let startedFeed = false;
  server.use(
    http.post('/auth/refresh', () => HttpResponse.json({ access_token: 'A' })),
    http.get('/me', async ({ request }) => {
      if (request.headers.get('Authorization') === 'Bearer B') return HttpResponse.json(user('B'));
      if (delayedMe) { startedMe = true; await meGate.promise; }
      return HttpResponse.json(user('A'));
    }),
    http.post('/auth/logout', () => new HttpResponse(null, { status: 204 })),
    http.post('/auth/login', () => HttpResponse.json({ access_token: 'B' })),
    http.post('/posts/x/like', async () => { startedLike = true; await likeGate.promise; return HttpResponse.json({ id: 'x', liked: true, likes: 4 }); }),
    http.get('/feed', async ({ request }) => {
      if (request.headers.get('Authorization') === 'Bearer B') return HttpResponse.error();
      startedFeed = true;
      await feedGate.promise;
      return HttpResponse.json({ pages: [{ posts: [{ id: 'A-secret' }], nextCursor: '' }], pageParams: [] });
    }),
  );
  const client = createQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}><AuthProvider>{children}</AuthProvider></QueryClientProvider>;
  const { result } = renderHook(() => ({ auth: useAuth(), like: useLike() }), { wrapper });
  await waitFor(() => expect(result.current.auth.user?.id).toBe('A'));
  client.setQueryData(['feed'], { pages: [{ posts: [{ id: 'x', likes: 0, liked: false }], nextCursor: '' }], pageParams: [] });
  client.setQueryData(['userPosts', 'A'], { pages: [{ posts: [{ id: 'a-secret', likes: 0, liked: false }], nextCursor: '' }], pageParams: [] });
  delayedMe = true;
  let oldMe!: Promise<void>;
  let oldLike!: Promise<unknown>;
  let oldFeed!: Promise<unknown>;
  act(() => {
    oldMe = result.current.auth.refreshUser();
    oldLike = result.current.like.mutateAsync({ id: 'x', liked: false }).catch(() => {});
  });
  await waitFor(() => { expect(startedMe).toBe(true); expect(startedLike).toBe(true); });
  act(() => { oldFeed = client.fetchQuery({ queryKey: ['feed'], queryFn: () => apiFetch('/feed') }).catch(() => {}); });
  await waitFor(() => expect(startedFeed).toBe(true));
  await act(async () => { await result.current.auth.logout(); await result.current.auth.login('B@example.com', 'password'); });
  expect(result.current.auth.user?.id).toBe('B');
  expect(client.getQueryData(['feed'])).toBeUndefined();
  expect(client.getQueryData(['userPosts', 'A'])).toBeUndefined();
  await act(async () => { await client.fetchQuery({ queryKey: ['feed'], queryFn: () => apiFetch('/feed'), retry: false }).catch(() => {}); });
  expect(client.getQueryData(['feed'])).toBeUndefined();
  await act(async () => { meGate.release(); likeGate.release(); feedGate.release(); await Promise.all([oldMe, oldLike, oldFeed]); });
  expect(result.current.auth.user?.id).toBe('B');
  expect(tokenStore.get()).toBe('B');
  expect(client.getQueryData(['post', 'x'])).toBeUndefined();
  expect(client.getQueryData(['feed'])).toBeUndefined();
});

test('bootstrap с повторным 401 завершается anon', async () => {
  server.use(http.post('/auth/refresh', () => HttpResponse.json({ access_token: 'A' })), http.get('/me', () => new HttpResponse(null, { status: 401 })));
  const client = createQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}><AuthProvider>{children}</AuthProvider></QueryClientProvider>;
  const { result } = renderHook(useAuth, { wrapper });
  await waitFor(() => expect(result.current.status).toBe('anon'));
});
