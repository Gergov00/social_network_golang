import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query';
import { postsApi } from '../../api/posts';
import type { FeedPage, PostDTO } from '../../api/types';
import { userPostsKey } from '../profile/useUserPosts';

export const FEED_KEY = ['feed'] as const;

type ListCache = { pages: FeedPage[]; pageParams: unknown[] };

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

function prependToFirstPage(old: ListCache | undefined, post: PostDTO): ListCache | undefined {
  if (!old) return old;
  const [first, ...rest] = old.pages;
  return { ...old, pages: [{ ...first, posts: [post, ...first.posts] }, ...rest] };
}

// Новый пост встаёт в начало первой страницы. Не invalidate: он сбросил бы
// все подгруженные страницы и вернул пользователя в начало ленты.
//
// Патчим ДВА кэша: ленту и СВОЮ вкладку «Записи» (userPostsKey({kind:'me'})) —
// иначе публикация с ленты и переход в /me показывает список без только что
// опубликованного поста, пока не сработает рефетч. В чужие
// ['userPosts', handle]-кэши намеренно НЕ лезем (в отличие от широкого
// listFilter в useLike.ts, который патчит уже существующие элементы по id):
// это был бы новый элемент, а не патч существующего, и «прописать» свой
// свежий пост в закэшированный список чужого профиля — баг, а не фикс.
export function usePrependPost() {
  const queryClient = useQueryClient();

  return (post: PostDTO) => {
    queryClient.setQueryData<ListCache>(FEED_KEY, (old) => prependToFirstPage(old, post));
    queryClient.setQueryData<ListCache>(userPostsKey({ kind: 'me' }), (old) => prependToFirstPage(old, post));
  };
}
