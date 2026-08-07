import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import { postsApi } from '../../api/posts';
import type { FeedPage, PostDTO } from '../../api/types';

export const FEED_KEY = ['feed'] as const;

export function useFeed() {
  return useInfiniteQuery({
    queryKey: FEED_KEY,
    queryFn: ({ pageParam }) => postsApi.getFeed(pageParam),
    initialPageParam: undefined as string | undefined,
    // nextCursor === '' означает «страниц больше нет». Вернуть отсюда пустую
    // строку вместо undefined = бесконечная догрузка одной и той же страницы.
    getNextPageParam: (last: FeedPage) => last.nextCursor || undefined,
  });
}

// Новый пост встаёт в начало первой страницы. Не invalidate: он сбросил бы
// все подгруженные страницы и вернул пользователя в начало ленты.
export function usePrependPost() {
  const queryClient = useQueryClient();

  return (post: PostDTO) => {
    queryClient.setQueryData<{ pages: FeedPage[]; pageParams: unknown[] }>(FEED_KEY, (old) => {
      if (!old) return old;
      const [first, ...rest] = old.pages;
      return {
        ...old,
        pages: [{ ...first, posts: [post, ...first.posts] }, ...rest],
      };
    });
  };
}
