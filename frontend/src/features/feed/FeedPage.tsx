import { useEffect, useRef } from 'react';
import { Empty } from '../../ui/Empty';
import { ErrorBox } from '../../ui/ErrorBox';
import { Spinner } from '../../ui/Spinner';
import { Composer } from './Composer';
import { PostCard } from './PostCard';
import { useFeed } from './useFeed';

export function FeedPage() {
  const feed = useFeed();
  const sentinel = useRef<HTMLDivElement | null>(null);

  // Бесконечный скролл: догружаем, когда «маячок» под лентой попал в вид.
  // В jsdom IntersectionObserver заглушён и не срабатывает — поэтому ниже
  // остаётся кнопка «Ещё», через неё пагинация и тестируется.
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !feed.hasNextPage) return;

    const observer = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && !feed.isFetchingNextPage) {
        void feed.fetchNextPage();
      }
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [feed.hasNextPage, feed.isFetchingNextPage, feed.fetchNextPage, feed]);

  const posts = feed.data?.pages.flatMap((page) => page.posts) ?? [];

  return (
    <div className="screen">
      <div className="center-col">
        <div className="feed">
          <Composer />

          {feed.isPending && <Spinner />}

          {feed.isError && (
            <ErrorBox message="Не удалось загрузить ленту" onRetry={() => void feed.refetch()} />
          )}

          {feed.isSuccess && posts.length === 0 && (
            <Empty title="Пока пусто" text="Напишите первую запись — она появится здесь." />
          )}

          {posts.map((post) => (
            <PostCard key={post.id} post={post} />
          ))}

          <div ref={sentinel} />

          {feed.hasNextPage && (
            <button
              className="btn btn-ghost"
              type="button"
              onClick={() => void feed.fetchNextPage()}
              disabled={feed.isFetchingNextPage}
            >
              {feed.isFetchingNextPage ? 'Загружаем…' : 'Ещё'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
