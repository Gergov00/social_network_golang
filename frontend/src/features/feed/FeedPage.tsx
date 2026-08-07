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

  // Вынесено из `feed` в отдельные переменные нарочно: TanStack Query
  // возвращает новый объект результата почти на любое изменение состояния
  // запроса (включая фоновые рефетчи). Если положить в зависимости эффекта
  // весь `feed`, наблюдатель будет пересоздаваться гораздо чаще, чем реально
  // меняются эти три поля — а IntersectionObserver репортит пересечение сразу
  // при observe(), так что маячок в зоне видимости мог бы ретриггерить
  // догрузку без реального скролла. fetchNextPage у TanStack стабильна по
  // ссылке между рендерами.
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = feed;

  // Бесконечный скролл: догружаем, когда «маячок» под лентой попал в вид.
  // В jsdom IntersectionObserver заглушён и не срабатывает — поэтому ниже
  // остаётся кнопка «Ещё», через неё пагинация и тестируется.
  useEffect(() => {
    const node = sentinel.current;
    if (!node || !hasNextPage) return;

    const observer = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && !isFetchingNextPage) {
        void fetchNextPage();
      }
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

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
