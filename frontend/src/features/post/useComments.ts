import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { commentsApi } from '../../api/comments';
import { tokenStore } from '../../api/tokenStore';
import { assertSession } from '../../api/client';
import { lockMutation } from '../../api/mutationLock';
import type { CommentDTO, CommentsPage, PostDTO } from '../../api/types';
import { listFilter, patchList, type ListCache } from '../feed/useLike';

type CommentsCache = { pages: CommentsPage[]; pageParams: unknown[] };

export const commentsKey = (postId: string) => ['comments', postId] as const;

export function useComments(postId: string) {
  return useInfiniteQuery({
    queryKey: commentsKey(postId),
    queryFn: ({ pageParam }) => commentsApi.list(postId, pageParam),
    initialPageParam: undefined as string | undefined,
    // nextCursor === '' значит «страниц больше нет».
    getNextPageParam: (last: CommentsPage) => last.nextCursor || undefined,
    enabled: postId !== '',
  });
}

// Новый комментарий встаёт в КОНЕЦ последней страницы: ветка отсортирована
// по возрастанию времени, свежая реплика — самая нижняя. Не invalidate:
// он сбросил бы подгруженные страницы и увёл бы пользователя в начало ветки.
export function useAddComment(postId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (text: string) => commentsApi.create(postId, text),
    onMutate: () => tokenStore.epoch(),
    onSuccess: (comment, _text, epoch) => {
      if (epoch !== tokenStore.epoch()) return;
      queryClient.setQueryData<CommentsCache>(commentsKey(postId), (old) => {
        if (!old) return old;
        const pages = old.pages.slice();
        const last = pages[pages.length - 1];
        pages[pages.length - 1] = { ...last, comments: [...last.comments, comment] };
        return { ...old, pages };
      });

      // Счётчик на карточке поста двигаем сами: сервер вернул комментарий,
      // а не пост, и без этого число под записью отстанет до перезагрузки.
      // Типизируем как PostDTO целиком (как useLike.ts:58), а не узким
      // { comments: number } — там же лежат liked/likes, которые никто не
      // должен случайно затереть неполным объектом.
      const updated = queryClient.setQueryData<PostDTO>(['post', postId], (old) =>
        old ? { ...old, comments: old.comments + 1 } : old,
      );

      // Пост живёт сразу в нескольких кэшах (лента, «Записи» своего и
      // чужого профиля) — patch только ['post', id] оставил бы счётчик в
      // списках устаревшим до перезагрузки. listFilter — тот же предикат,
      // что useLike.ts применяет для лайков (см. useLike.ts:12-15).
      if (updated) {
        queryClient.setQueriesData<ListCache>(listFilter, (old) => patchList(old, postId, { comments: updated.comments }));
      }
    },
  });
}

function patchComments(cache: CommentsCache | undefined, id: string, patch: Partial<CommentDTO>) {
  if (!cache) return cache;
  return {
    ...cache,
    pages: cache.pages.map((page) => ({
      ...page,
      comments: page.comments.map((c) => (c.id === id ? { ...c, ...patch } : c)),
    })),
  };
}

export function useCommentLike(postId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, liked }: { id: string; liked: boolean }) =>
      liked ? commentsApi.unlike(id) : commentsApi.like(id),

    // Оптимистично: счётчик двигается до ответа сервера.
    onMutate: async ({ id, liked }) => {
      const epoch = tokenStore.epoch();
      const release = await lockMutation(queryClient, `${epoch}:comment:${id}`);
      if (epoch !== tokenStore.epoch()) { release(); assertSession(epoch); }
      await queryClient.cancelQueries({ queryKey: commentsKey(postId) });
      if (epoch !== tokenStore.epoch()) { release(); assertSession(epoch); }
      const snapshot = queryClient.getQueryData<CommentsCache>(commentsKey(postId));

      queryClient.setQueryData<CommentsCache>(commentsKey(postId), (old) => {
        const current = old?.pages.flatMap((p) => p.comments).find((c) => c.id === id);
        if (!current) return old;
        return patchComments(old, id, {
          liked: !liked,
          likes: current.likes + Number(!liked) - Number(current.liked),
        });
      });

      return { snapshot, epoch, release };
    },

    // Откат: иначе на экране останется число, которого нет в базе.
    onError: (_err, { id }, context) => {
      if (!context || context.epoch !== tokenStore.epoch()) return;
      const previous = context.snapshot?.pages.flatMap((page) => page.comments).find((c) => c.id === id);
      if (previous) queryClient.setQueryData<CommentsCache>(commentsKey(postId), (old) => patchComments(old, id, { liked: previous.liked, likes: previous.likes }));
    },

    // Сервер вернул комментарий целиком — его числа и есть истина.
    onSuccess: (fresh, _vars, context) => {
      if (!context || context.epoch !== tokenStore.epoch()) return;
      queryClient.setQueryData<CommentsCache>(commentsKey(postId), (old) =>
        patchComments(old, fresh.id, { liked: fresh.liked, likes: fresh.likes }),
      );
    },
    onSettled: (_data, _error, _vars, context) => context?.release(),
  });
}
