import { useState } from 'react';
import { useAuth } from '../../auth/AuthContext';
import { Avatar } from '../../ui/Avatar';
import { ErrorBox } from '../../ui/ErrorBox';
import { useAddComment } from './useComments';

// Столько же, сколько принимает post-service (service.MaxCommentRunes).
const MAX_RUNES = 2000;

export function CommentComposer({ postId }: { postId: string }) {
  const { user } = useAuth();
  const [text, setText] = useState('');
  const add = useAddComment(postId);

  const trimmed = text.trim();
  const tooLong = [...trimmed].length > MAX_RUNES;

  const submit = () => {
    if (!trimmed || tooLong || add.isPending) return;
    // Очищаем только в onSuccess: при ошибке текст обязан остаться, чтобы
    // пользователю не пришлось набирать его заново.
    add.mutate(trimmed, { onSuccess: () => setText('') });
  };

  return (
    <div className="card comment-composer">
      <div className="composer-top">
        <Avatar id={user?.id ?? ''} name={user?.name ?? ''} handle={user?.handle ?? ''} size={36} />
        <input
          value={text}
          placeholder="Написать комментарий…"
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            // Enter отправляет — как в прототипе.
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault();
              submit();
            }
          }}
        />
        <button
          className="btn btn-primary btn-sm"
          type="button"
          onClick={submit}
          disabled={!trimmed || tooLong || add.isPending}
        >
          {add.isPending ? 'Отправляем…' : 'Отправить'}
        </button>
      </div>

      {tooLong && <span className="hint">Длиннее {MAX_RUNES} символов</span>}

      {add.isError && (
        <ErrorBox message="Не удалось отправить комментарий. Попробуйте ещё раз." onRetry={submit} />
      )}
    </div>
  );
}
