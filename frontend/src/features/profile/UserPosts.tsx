import { Empty } from '../../ui/Empty';
import { ErrorBox } from '../../ui/ErrorBox';
import { Spinner } from '../../ui/Spinner';
import { PostCard } from '../feed/PostCard';
import { useUserPosts, type PostsOwner } from './useUserPosts';

export function UserPosts({ owner }: { owner: PostsOwner }) {
  const query = useUserPosts(owner);
  const posts = query.data?.pages.flatMap((page) => page.posts) ?? [];

  return (
    <div className="feed">
      {query.isPending && <Spinner />}

      {query.isError && (
        <ErrorBox message="Не удалось загрузить записи" onRetry={() => void query.refetch()} />
      )}

      {query.isSuccess && posts.length === 0 && (
        <Empty
          title="Записей пока нет"
          text={owner.kind === 'me' ? 'Опубликуйте первую — она появится здесь.' : undefined}
        />
      )}

      {posts.map((post) => (
        <PostCard key={post.id} post={post} />
      ))}

      {/* Бесконечного скролла тут нет намеренно: вкладка внутри профиля, а не
          отдельный экран — кнопка предсказуемее и не конфликтует с наблюдателем
          ленты, если пользователь вернётся на неё назад. */}
      {query.hasNextPage && (
        <button
          className="btn btn-ghost"
          type="button"
          onClick={() => void query.fetchNextPage()}
          disabled={query.isFetchingNextPage}
        >
          {query.isFetchingNextPage ? 'Загружаем…' : 'Ещё'}
        </button>
      )}
    </div>
  );
}
