import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { postsApi } from '../../api/posts';
import { useAuth } from '../../auth/AuthContext';
import { Avatar } from '../../ui/Avatar';
import { ErrorBox } from '../../ui/ErrorBox';
import { Icon } from '../../ui/Icons';
import { usePrependPost } from './useFeed';

// Столько же, сколько принимает post-service (service.MaxTextRunes).
const MAX_RUNES = 5000;

export function Composer() {
  const { user } = useAuth();
  const prepend = usePrependPost();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState('');

  const publish = useMutation({
    mutationFn: (value: string) => postsApi.createPost(value),
    onSuccess: (post) => {
      prepend(post);
      setText('');
      setOpen(false);
    },
  });

  const trimmed = text.trim();
  const tooLong = [...trimmed].length > MAX_RUNES;

  const submit = () => {
    if (!trimmed || tooLong || publish.isPending) return;
    publish.mutate(trimmed);
  };

  return (
    <div className="card composer">
      <div className="composer-top">
        <Avatar id={user?.id ?? ''} name={user?.name ?? ''} handle={user?.handle ?? ''} size={46} />
        {open ? (
          <textarea
            autoFocus
            value={text}
            placeholder="Поделитесь мыслью…"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') submit();
            }}
          />
        ) : (
          // Реальный input с placeholder, а не текстовый div: доступный
          // признак («поле для поста») должен находиться по placeholder,
          // как ищет пользователь глазами и как ищет тест — findByPlaceholderText.
          <input
            className="composer-fake"
            placeholder="Поделитесь мыслью…"
            readOnly
            onClick={() => setOpen(true)}
            onFocus={() => setOpen(true)}
          />
        )}
      </div>

      {publish.isError && (
        <ErrorBox
          message="Не удалось опубликовать запись. Попробуйте ещё раз."
          onRetry={submit}
        />
      )}

      {open && (
        <div className="composer-actions">
          {/* Медиа и опросы бэкенд не поддерживает — кнопки неактивны. */}
          <button className="tool" type="button" disabled title="Скоро"><Icon name="image" /></button>
          <button className="tool" type="button" disabled title="Скоро"><Icon name="video" /></button>
          <button className="tool" type="button" disabled title="Скоро"><Icon name="poll" /></button>
          <span className="grow" />
          {tooLong && <span className="hint">Длиннее {MAX_RUNES} символов</span>}
          <button
            className="btn btn-ghost btn-sm"
            type="button"
            onClick={() => { setText(''); setOpen(false); publish.reset(); }}
          >
            Отмена
          </button>
          <button className="btn btn-primary btn-sm" type="button" onClick={submit} disabled={!trimmed || tooLong || publish.isPending}>
            {publish.isPending ? 'Публикуем…' : 'Опубликовать'}
          </button>
        </div>
      )}
    </div>
  );
}
