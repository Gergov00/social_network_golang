import { useMutation, useQueryClient } from '@tanstack/react-query';
import { postsApi } from '../../api/posts';
import type { FeedPage, PostDTO } from '../../api/types';
import { FEED_KEY } from './useFeed';

type FeedCache = { pages: FeedPage[]; pageParams: unknown[] };

function patchCache(cache: FeedCache | undefined, id: string, patch: Partial<PostDTO>) {
  if (!cache) return cache;
  return {
    ...cache,
    pages: cache.pages.map((page) => ({
      ...page,
      posts: page.posts.map((p) => (p.id === id ? { ...p, ...patch } : p)),
    })),
  };
}

export function useLike() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, liked }: { id: string; liked: boolean }) =>
      liked ? postsApi.unlikePost(id) : postsApi.likePost(id),

    // Оптимистично: счётчик двигается сразу, до ответа сервера.
    onMutate: async ({ id, liked }) => {
      await queryClient.cancelQueries({ queryKey: FEED_KEY });
      const snapshot = queryClient.getQueryData<FeedCache>(FEED_KEY);

      queryClient.setQueryData<FeedCache>(FEED_KEY, (old) => {
        const current = old?.pages.flatMap((p) => p.posts).find((p) => p.id === id);
        if (!current) return old;
        return patchCache(old, id, {
          liked: !liked,
          likes: current.likes + (liked ? -1 : 1),
        });
      });

      queryClient.setQueryData<PostDTO>(['post', id], (old) =>
        old ? { ...old, liked: !liked, likes: old.likes + (liked ? -1 : 1) } : old,
      );

      return { snapshot };
    },

    // Откат: без него неудачный лайк оставит на экране число, которого нет
    // в базе, и пользователь узнает об этом только после перезагрузки.
    onError: (_err, _vars, context) => {
      if (context?.snapshot) queryClient.setQueryData(FEED_KEY, context.snapshot);
    },

    // Сервер вернул пост целиком — берём его числа как истину в последней
    // инстанции: параллельные лайки других людей наш +1 не учитывал.
    onSuccess: (fresh) => {
      queryClient.setQueryData<FeedCache>(FEED_KEY, (old) =>
        patchCache(old, fresh.id, { liked: fresh.liked, likes: fresh.likes }),
      );
      queryClient.setQueryData(['post', fresh.id], fresh);
    },
  });
}
