import { apiFetch } from './client';
import type { FeedPage, PostDTO } from './types';

export const FEED_LIMIT = 20;

export const postsApi = {
  getFeed: (cursor?: string) => {
    const params = new URLSearchParams({ limit: String(FEED_LIMIT) });
    if (cursor) params.set('cursor', cursor);
    return apiFetch<FeedPage>(`/feed?${params.toString()}`);
  },

  getPost: (id: string) => apiFetch<PostDTO>(`/posts/${encodeURIComponent(id)}`),

  createPost: (text: string) =>
    apiFetch<PostDTO>('/posts', { method: 'POST', body: JSON.stringify({ text }) }),

  likePost: (id: string) =>
    apiFetch<PostDTO>(`/posts/${encodeURIComponent(id)}/like`, { method: 'POST' }),

  unlikePost: (id: string) =>
    apiFetch<PostDTO>(`/posts/${encodeURIComponent(id)}/like`, { method: 'DELETE' }),

  getMyPosts: (cursor?: string) => {
    const params = new URLSearchParams({ limit: String(FEED_LIMIT) });
    if (cursor) params.set('cursor', cursor);
    return apiFetch<FeedPage>(`/me/posts?${params.toString()}`);
  },

  getUserPosts: (handle: string, cursor?: string) => {
    const params = new URLSearchParams({ limit: String(FEED_LIMIT) });
    if (cursor) params.set('cursor', cursor);
    return apiFetch<FeedPage>(`/users/${encodeURIComponent(handle)}/posts?${params.toString()}`);
  },
};
