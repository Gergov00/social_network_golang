import { useInfiniteQuery } from '@tanstack/react-query';
import { postsApi } from '../../api/posts';
import type { FeedPage } from '../../api/types';

// Свой профиль и чужой — РАЗНЫЕ эндпоинты, а не один с параметром: своего
// handle на бэкенде спрашивать незачем, id вызывающего уже в токене.
export type PostsOwner = { kind: 'me' } | { kind: 'handle'; handle: string };

// Первый сегмент ключа общий для всех списков записей — по нему useLike
// находит и патчит их все разом (см. useLike.ts).
export const USER_POSTS_KEY = 'userPosts';

export function userPostsKey(owner: PostsOwner) {
  return [USER_POSTS_KEY, owner.kind === 'me' ? 'me' : owner.handle] as const;
}

export function useUserPosts(owner: PostsOwner) {
  return useInfiniteQuery({
    queryKey: userPostsKey(owner),
    queryFn: ({ pageParam }) =>
      owner.kind === 'me'
        ? postsApi.getMyPosts(pageParam)
        : postsApi.getUserPosts(owner.handle, pageParam),
    initialPageParam: undefined as string | undefined,
    // nextCursor === '' значит «страниц больше нет». Пустая строка вместо
    // undefined = бесконечная догрузка одной и той же страницы.
    getNextPageParam: (last: FeedPage) => last.nextCursor || undefined,
  });
}
