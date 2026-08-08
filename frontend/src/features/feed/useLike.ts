import { useMutation, useQueryClient, type Query } from '@tanstack/react-query';
import { postsApi } from '../../api/posts';
import type { FeedPage, PostDTO } from '../../api/types';
import { FEED_KEY } from './useFeed';
import { USER_POSTS_KEY } from '../profile/useUserPosts';

type ListCache = { pages: FeedPage[]; pageParams: unknown[] };

// Один пост живёт сразу в нескольких кэшах: лента, вкладка «Записи» своего
// профиля, вкладка «Записи» чужого. Патчить только ленту — значит оставить
// в остальных списках устаревший счётчик до перезагрузки страницы.
const listFilter = {
  predicate: (query: Query) =>
    query.queryKey[0] === FEED_KEY[0] || query.queryKey[0] === USER_POSTS_KEY,
};

function patchList(cache: ListCache | undefined, id: string, patch: Partial<PostDTO>) {
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
      // Отменяем ВСЁ, что патчим ниже, иначе фоновый рефетч перезапишет
      // оптимистичное значение прямо посреди мутации.
      await queryClient.cancelQueries(listFilter);
      await queryClient.cancelQueries({ queryKey: ['post', id] });

      const listSnapshots = queryClient.getQueriesData<ListCache>(listFilter);
      const postSnapshot = queryClient.getQueryData<PostDTO>(['post', id]);

      queryClient.setQueriesData<ListCache>(listFilter, (old) => {
        const current = findPost(old, id);
        if (!current) return old;
        return patchList(old, id, {
          liked: !liked,
          likes: current.likes + (liked ? -1 : 1),
        });
      });

      queryClient.setQueryData<PostDTO>(['post', id], (old) =>
        old ? { ...old, liked: !liked, likes: old.likes + (liked ? -1 : 1) } : old,
      );

      return { listSnapshots, postSnapshot };
    },

    // Откат: без него неудачный лайк оставит на экране число, которого нет
    // в базе, и пользователь узнает об этом только после перезагрузки.
    onError: (_err, { id }, context) => {
      context?.listSnapshots.forEach(([key, data]) => queryClient.setQueryData(key, data));
      if (context?.postSnapshot) queryClient.setQueryData(['post', id], context.postSnapshot);
    },

    // Сервер вернул пост целиком — берём его числа как истину в последней
    // инстанции: параллельные лайки других людей наш +1 не учитывал.
    onSuccess: (fresh) => {
      queryClient.setQueriesData<ListCache>(listFilter, (old) =>
        patchList(old, fresh.id, { liked: fresh.liked, likes: fresh.likes }),
      );
      queryClient.setQueryData(['post', fresh.id], fresh);
    },
  });
}
