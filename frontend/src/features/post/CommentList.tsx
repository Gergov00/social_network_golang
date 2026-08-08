import type { CommentDTO } from '../../api/types';
import { Avatar } from '../../ui/Avatar';
import { Empty } from '../../ui/Empty';
import { ErrorBox } from '../../ui/ErrorBox';
import { Icon } from '../../ui/Icons';
import { Spinner } from '../../ui/Spinner';
import { relativeTime } from '../feed/PostCard';
import { useComments, useCommentLike } from './useComments';

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
      <div className="bubble">
        <div className="post-name">{name}</div>
        <div>{comment.text}</div>
        <div className="post-sub">
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
          <button className="iact" type="button" disabled title="Скоро">Ответить</button>
        </div>
      </div>
    </div>
  );
}

export function CommentList({ postId }: { postId: string }) {
  const query = useComments(postId);
  const comments = query.data?.pages.flatMap((page) => page.comments) ?? [];

  return (
    <div className="comments">
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
