import type { CommentDTO } from '../../api/types';
import { Avatar } from '../../ui/Avatar';
import { Empty } from '../../ui/Empty';
import { ErrorBox } from '../../ui/ErrorBox';
import { Icon } from '../../ui/Icons';
import { Spinner } from '../../ui/Spinner';
import { relativeTime } from '../feed/PostCard';
import { useComments, useCommentLike } from './useComments';

function compareCreatedAt(a: string, b: string): number {
  // Date.parse truncates sub-millisecond digits from Go's RFC3339Nano.
  // Compare UTC whole seconds, then the normalized nanosecond fraction.
  const fraction = /\.(\d+)(?=Z|[+-])/;
  const seconds = Date.parse(a.replace(fraction, '')) - Date.parse(b.replace(fraction, ''));
  if (seconds) return seconds;
  const left = (a.match(fraction)?.[1] ?? '').padEnd(9, '0');
  const right = (b.match(fraction)?.[1] ?? '').padEnd(9, '0');
  return left < right ? -1 : left > right ? 1 : 0;
}

function CommentRow({ comment, postId }: { comment: CommentDTO; postId: string }) {
  const like = useCommentLike(postId);
  const author = comment.author;

  // author === null — законное состояние (см. types.ts): рисуем заглушку,
  // реплику не прячем.
  const name = author?.name?.trim() || author?.handle || 'Неизвестный автор';

  return (
    <div className="comment">
      <Avatar
        id={comment.authorId}
        name={author?.name ?? ''}
        handle={author?.handle ?? comment.authorId}
        size={36}
      />
      <div className="c-bubble">
        <span className="c-name">{name}</span>
        <span className="c-text">{comment.text}</span>
        <div className="c-meta">
          {relativeTime(comment.createdAt)}
          <button
            className={`iact${comment.liked ? ' liked' : ''}`}
            type="button"
            aria-label={comment.liked ? 'Не нравится комментарий' : 'Нравится комментарий'}
            aria-pressed={comment.liked}
            onClick={() => like.mutate({ id: comment.id, liked: comment.liked })}
          >
            <Icon name="heart" fill={comment.liked} size={14} /> {comment.likes}
          </button>
          {/* Ответов на комментарии бэкенд не умеет: плоская ветка. */}
          <button className="c-act" type="button" disabled title="Скоро">Ответить</button>
        </div>
      </div>
    </div>
  );
}

export function CommentList({ postId }: { postId: string }) {
  const query = useComments(postId);
  // Дедуп по id обязателен: useAddComment дописывает свежий комментарий в
  // конец загруженной страницы, не сдвигая её nextCursor, поэтому тот же
  // комментарий придёт ещё раз со следующей серверной страницей. Ветка ASC —
  // в отличие от ленты (DESC), где добавленный в начало пост повторно
  // прийти не может.
  const comments = [...new Map(
    (query.data?.pages.flatMap((page) => page.comments) ?? []).map((c) => [c.id, c]),
  ).values()].sort((a, b) => {
    const time = compareCreatedAt(a.createdAt, b.createdAt);
    return time || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
  });

  return (
    <div className="comment-list">
      {query.isPending && <Spinner />}

      {query.isError && (
        <ErrorBox message="Не удалось загрузить комментарии" onRetry={() => void query.refetch()} />
      )}

      {query.isSuccess && comments.length === 0 && (
        <Empty title="Комментариев пока нет" text="Будьте первым." />
      )}

      {comments.map((comment) => (
        <CommentRow key={comment.id} comment={comment} postId={postId} />
      ))}

      {query.hasNextPage && (
        <button
          className="btn btn-ghost"
          type="button"
          onClick={() => void query.fetchNextPage()}
          disabled={query.isFetchingNextPage}
        >
          {query.isFetchingNextPage ? 'Загружаем…' : 'Ещё комментарии'}
        </button>
      )}
    </div>
  );
}
