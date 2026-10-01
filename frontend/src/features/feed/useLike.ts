import { useMutation, useQueryClient, type Query } from '@tanstack/react-query';
import { postsApi } from '../../api/posts';
import { tokenStore } from '../../api/tokenStore';
import { assertSession } from '../../api/client';
import { lockMutation } from '../../api/mutationLock';
import type { FeedPage, PostDTO } from '../../api/types';
import { FEED_KEY } from './useFeed';
import { USER_POSTS_KEY } from '../profile/useUserPosts';

export type ListCache = { pages: FeedPage[]; pageParams: unknown[] };

// Один пост живёт сразу в нескольких кэшах: лента, вкладка «Записи» своего
// профиля, вкладка «Записи» чужого. Патчить только ленту — значит оставить
// в остальных списках устаревший счётчик до перезагрузки страницы.
// Экспортируется: useComments.ts переиспользует его для синхронизации
// счётчика комментариев по той же причине.
export const listFilter = {
  predicate: (query: Query) =>
    query.queryKey[0] === FEED_KEY[0] || query.queryKey[0] === USER_POSTS_KEY,
};

export function patchList(cache: ListCache | undefined, id: string, patch: Partial<PostDTO>) {
  if (!cache) return cache;
  return {
    ...cache,
    pages: cache.pages.map((page) => ({
      ...page,
      posts: page.posts.map((p) => (p.id === id ? { ...p, ...patch } : p)),
    })),
  };
}

function findPost(cache: ListCache | undefined, id: string): PostDTO | undefined {
  return cache?.pages.flatMap((page) => page.posts).find((p) => p.id === id);
}

export function useLike() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, liked }: { id: string; liked: boolean }) =>
      liked ? postsApi.unlikePost(id) : postsApi.likePost(id),

    // Оптимистично: счётчик двигается сразу, до ответа сервера.
    onMutate: async ({ id, liked }) => {
      const epoch = tokenStore.epoch();
      const release = await lockMutation(queryClient, `${epoch}:post:${id}`);
      if (epoch !== tokenStore.epoch()) { release(); assertSession(epoch); }
      // Отменяем ВСЁ, что патчим ниже, иначе фоновый рефетч перезапишет
      // оптимистичное значение прямо посреди мутации.
      await queryClient.cancelQueries(listFilter);
      await queryClient.cancelQueries({ queryKey: ['post', id] });
      if (epoch !== tokenStore.epoch()) { release(); assertSession(epoch); }

      const listSnapshots = queryClient.getQueriesData<ListCache>(listFilter);
      const postSnapshot = queryClient.getQueryData<PostDTO>(['post', id]);

      queryClient.setQueriesData<ListCache>(listFilter, (old) => {
        const current = findPost(old, id);
        if (!current) return old;
        return patchList(old, id, {
          liked: !liked,
          likes: current.likes + Number(!liked) - Number(current.liked),
        });
      });

      queryClient.setQueryData<PostDTO>(['post', id], (old) =>
        old ? { ...old, liked: !liked, likes: old.likes + Number(!liked) - Number(old.liked) } : old,
      );

      return { listSnapshots, postSnapshot, epoch, release };
    },

    // Откат: без него неудачный лайк оставит на экране число, которого нет
    // в базе, и пользователь узнает об этом только после перезагрузки.
    onError: (_err, { id }, context) => {
      if (!context || context.epoch !== tokenStore.epoch()) return;
      context.listSnapshots.forEach(([key, data]) => {
        const previous = findPost(data, id);
        if (previous) queryClient.setQueryData<ListCache>(key, (old) => patchList(old, id, { liked: previous.liked, likes: previous.likes }));
      });
      const previous = context.postSnapshot;
      if (previous) queryClient.setQueryData<PostDTO>(['post', id], (old) => old ? { ...old, liked: previous.liked, likes: previous.likes } : old);
    },

    // Сервер вернул пост целиком — берём его числа как истину в последней
    // инстанции: параллельные лайки других людей наш +1 не учитывал.
    onSuccess: (fresh, _vars, context) => {
      if (!context || context.epoch !== tokenStore.epoch()) return;
      queryClient.setQueriesData<ListCache>(listFilter, (old) =>
        patchList(old, fresh.id, { liked: fresh.liked, likes: fresh.likes }),
      );
      queryClient.setQueryData<PostDTO>(['post', fresh.id], (old) => old ? { ...old, liked: fresh.liked, likes: fresh.likes } : fresh);
    },
    onSettled: (_data, _error, _vars, context) => context?.release(),
  });
}
