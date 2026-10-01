import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { http, HttpResponse } from 'msw';
import { expect, test } from 'vitest';
import { server } from '../../test/server';
import { createQueryClient } from '../../app/providers';
import { useLike, type ListCache } from './useLike';
import { useAddComment, useCommentLike, commentsKey } from '../post/useComments';
import type { CommentDTO, PostDTO } from '../../api/types';

const post = (id: string): PostDTO => ({ id, authorId: 'a', author: null, text: id, likes: 0, liked: false, comments: 0, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' });
const comment = (id: string): CommentDTO => ({ ...post(id), postId: 'x' });
function setup() {
  const client = createQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  client.setQueryData(['feed'], { pages: [{ posts: [post('x'), post('y')], nextCursor: '' }], pageParams: [undefined] });
  client.setQueryData(['post', 'x'], post('x'));
  client.setQueryData(commentsKey('x'), { pages: [{ comments: [comment('x'), comment('y')], nextCursor: '' }], pageParams: [undefined] });
  return { client, wrapper };
}
function deferred() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => { release = resolve; });
  return { promise, release };
}

test('ошибка лайка X сохраняет успешный Y и новый комментарий у X', async () => {
  const { client, wrapper } = setup();
  const gate = deferred();
  server.use(
    http.post('/posts/x/like', async () => { await gate.promise; return new HttpResponse(null, { status: 500 }); }),
    http.post('/posts/y/like', () => HttpResponse.json({ ...post('y'), liked: true, likes: 9 })),
    http.post('/posts/x/comments', () => HttpResponse.json(comment('new'))),
  );
  const { result } = renderHook(() => ({ like: useLike(), add: useAddComment('x') }), { wrapper });
  let first!: Promise<unknown>;
  act(() => { first = result.current.like.mutateAsync({ id: 'x', liked: false }).catch(() => {}); });
  await waitFor(() => expect(client.getQueryData<PostDTO>(['post', 'x'])?.liked).toBe(true));
  await act(async () => { await result.current.like.mutateAsync({ id: 'y', liked: false }); await result.current.add.mutateAsync('new'); });
  await act(async () => { gate.release(); await first; });
  const posts = client.getQueryData<ListCache>(['feed'])!.pages[0].posts;
  expect(posts[1]).toMatchObject({ liked: true, likes: 9 });
  expect(posts[0]).toMatchObject({ liked: false, likes: 0, comments: 1 });
  expect(client.getQueryData<PostDTO>(['post', 'x'])).toMatchObject({ comments: 1, liked: false });
});

test('rollback комментария X сохраняет успешный Y и добавленную реплику', async () => {
  const { client, wrapper } = setup();
  const gate = deferred();
  server.use(
    http.post('/comments/x/like', async () => { await gate.promise; return new HttpResponse(null, { status: 500 }); }),
    http.post('/comments/y/like', () => HttpResponse.json({ ...comment('y'), liked: true, likes: 9 })),
    http.post('/posts/x/comments', () => HttpResponse.json(comment('new'))),
  );
  const { result } = renderHook(() => ({ like: useCommentLike('x'), add: useAddComment('x') }), { wrapper });
  let first!: Promise<unknown>;
  act(() => { first = result.current.like.mutateAsync({ id: 'x', liked: false }).catch(() => {}); });
  await waitFor(() => expect(client.getQueryData<{ pages: { comments: CommentDTO[] }[] }>(commentsKey('x'))?.pages[0].comments[0].liked).toBe(true));
  await act(async () => { await result.current.like.mutateAsync({ id: 'y', liked: false }); await result.current.add.mutateAsync('new'); });
  await act(async () => { gate.release(); await first; });
  expect(client.getQueryData<{ pages: { comments: CommentDTO[] }[] }>(commentsKey('x'))!.pages[0].comments).toEqual([comment('x'), { ...comment('y'), likes: 9, liked: true }, comment('new')]);
});

test.each(['post', 'comment'] as const)('лайки одного %s сериализуются, второй rollback сохраняет успех первого', async (kind) => {
  const { client, wrapper } = setup();
  const gate = deferred();
  let calls = 0;
  const endpoint = kind === 'post' ? '/posts/x/like' : '/comments/x/like';
  server.use(http.post(endpoint, async () => {
    calls += 1;
    if (calls === 1) { await gate.promise; return HttpResponse.json({ ...(kind === 'post' ? post('x') : comment('x')), liked: true, likes: 9 }); }
    return new HttpResponse(null, { status: 500 });
  }));
  const { result } = renderHook(() => ({ post: useLike(), comment: useCommentLike('x') }), { wrapper });
  const mutation = () => result.current[kind];
  let first!: Promise<unknown>;
  let second!: Promise<unknown>;
  act(() => { first = mutation().mutateAsync({ id: 'x', liked: false }); });
  await waitFor(() => expect(calls).toBe(1));
  act(() => { second = mutation().mutateAsync({ id: 'x', liked: false }).catch(() => {}); });
  await new Promise((resolve) => setTimeout(resolve, 30));
  expect(calls).toBe(1);
  await act(async () => { gate.release(); await Promise.all([first, second]); });
  if (kind === 'post') expect(client.getQueryData<PostDTO>(['post', 'x'])).toMatchObject({ liked: true, likes: 9 });
  else expect(client.getQueryData<{ pages: { comments: CommentDTO[] }[] }>(commentsKey('x'))!.pages[0].comments[0]).toMatchObject({ liked: true, likes: 9 });
});

test.each(['post', 'comment'] as const)('queued unlike %s после ошибки like не делает счётчик отрицательным', async (kind) => {
  const { client, wrapper } = setup();
  const firstGate = deferred();
  const secondGate = deferred();
  let secondStarted = false;
  const endpoint = kind === 'post' ? '/posts/x/like' : '/comments/x/like';
  server.use(
    http.post(endpoint, async () => { await firstGate.promise; return new HttpResponse(null, { status: 500 }); }),
    http.delete(endpoint, async () => { secondStarted = true; await secondGate.promise; return HttpResponse.json(kind === 'post' ? post('x') : comment('x')); }),
  );
  const { result } = renderHook(() => ({ post: useLike(), comment: useCommentLike('x') }), { wrapper });
  const cached = () => kind === 'post' ? client.getQueryData<PostDTO>(['post', 'x'])! : client.getQueryData<{ pages: { comments: CommentDTO[] }[] }>(commentsKey('x'))!.pages[0].comments[0];
  let first!: Promise<unknown>;
  let second!: Promise<unknown>;
  act(() => { first = result.current[kind].mutateAsync({ id: 'x', liked: false }).catch(() => {}); });
  await waitFor(() => expect(cached().liked).toBe(true));
  act(() => { second = result.current[kind].mutateAsync({ id: 'x', liked: true }); });
  await act(async () => { firstGate.release(); await first; });
  await waitFor(() => expect(secondStarted).toBe(true));
  const optimistic = { liked: cached().liked, likes: cached().likes };
  await act(async () => { secondGate.release(); await second; });
  expect(optimistic).toEqual({ liked: false, likes: 0 });
});
