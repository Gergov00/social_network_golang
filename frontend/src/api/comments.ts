import { apiFetch } from './client';
import type { CommentDTO, CommentsPage } from './types';

export const COMMENTS_LIMIT = 20;

export const commentsApi = {
  list: (postId: string, cursor?: string) => {
    const params = new URLSearchParams({ limit: String(COMMENTS_LIMIT) });
    if (cursor) params.set('cursor', cursor);
    return apiFetch<CommentsPage>(`/posts/${encodeURIComponent(postId)}/comments?${params.toString()}`);
  },

  create: (postId: string, text: string) =>
    apiFetch<CommentDTO>(`/posts/${encodeURIComponent(postId)}/comments`, {
      method: 'POST',
      body: JSON.stringify({ text }),
    }),

  like: (id: string) =>
    apiFetch<CommentDTO>(`/comments/${encodeURIComponent(id)}/like`, { method: 'POST' }),

  unlike: (id: string) =>
    apiFetch<CommentDTO>(`/comments/${encodeURIComponent(id)}/like`, { method: 'DELETE' }),
};
