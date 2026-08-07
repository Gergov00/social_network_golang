import { Link } from 'react-router-dom';
import type { PostDTO } from '../../api/types';
import { Avatar } from '../../ui/Avatar';
import { Icon } from '../../ui/Icons';
import { useLike } from './useLike';

// «2 ч», «5 мин» — как в прототипе, где время приходило готовой строкой.
export function relativeTime(iso: string, now = Date.now()): string {
  const diff = Math.max(0, now - new Date(iso).getTime());
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return 'только что';
  if (minutes < 60) return `${minutes} мин`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} ч`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days} д`;
  return new Date(iso).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' });
}

function renderText(text: string) {
  return text.split(/(#[\wа-яё]+)/gi).map((part, i) =>
    part.startsWith('#') ? (
      <span className="tag" key={i}>{part}</span>
    ) : (
      <span key={i}>{part}</span>
    ),
  );
}

export function PostCard({ post, detail = false }: { post: PostDTO; detail?: boolean }) {
  const like = useLike();
  const author = post.author;

  // author === null — законное состояние (см. types.ts). Показываем
  // «Неизвестный автор» и не даём ссылку, но пост рисуем как обычно.
  const name = author?.name?.trim() || author?.handle || 'Неизвестный автор';
  const handle = author ? `@${author.handle}` : '';

  const head = (
    <>
      <Avatar
        id={post.authorId}
        name={author?.name ?? ''}
        handle={author?.handle ?? post.authorId}
        size={46}
      />
      <div className="meta">
        <div className="post-name">
          {name}
          {author?.verified && <span className="badge-v"><Icon name="check" size={15} /></span>}
        </div>
        <div className="post-sub">
          {handle} {handle && '· '}
          {relativeTime(post.createdAt)}
        </div>
      </div>
    </>
  );

  return (
    <article className="card post">
      <div className="post-head">
        {author ? (
          <Link className="pclick" to={`/u/${author.handle}`}>{head}</Link>
        ) : (
          <div className="pclick" style={{ cursor: 'default' }}>{head}</div>
        )}
        <button className="tool" type="button" disabled title="Скоро">
          <Icon name="more" />
        </button>
      </div>

      {detail ? (
        <div className="post-body">{renderText(post.text)}</div>
      ) : (
        <Link className="post-body clickable" to={`/post/${post.id}`} style={{ display: 'block', color: 'inherit', textDecoration: 'none' }}>
          {renderText(post.text)}
        </Link>
      )}

      <div className="post-stats">
        <button
          className={`iact${post.liked ? ' liked' : ''}`}
          type="button"
          aria-label={post.liked ? 'Не нравится' : 'Нравится'}
          aria-pressed={post.liked}
          onClick={() => like.mutate({ id: post.id, liked: post.liked })}
        >
          <Icon name="heart" fill={post.liked} /> {post.likes}
        </button>

        {/* Комментарии, репосты и закладки бэкенд не умеет. Кнопки на месте,
            но неактивны: интерфейс не должен обещать несуществующего. */}
        <button className="iact" type="button" aria-label="Комментировать" disabled title="Скоро">
          <Icon name="comment" />
        </button>
        <button className="iact" type="button" aria-label="Репост" disabled title="Скоро">
          <Icon name="share" />
        </button>
        <span className="spacer" />
        <button className="iact" type="button" aria-label="В закладки" disabled title="Скоро">
          <Icon name="bookmark" />
        </button>
      </div>
    </article>
  );
}
