import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { ApiError } from '../../api/client';
import { usersApi } from '../../api/users';
import { tokenStore } from '../../api/tokenStore';
import type { UpdateMePayload, UserDTO } from '../../api/types';

type Props = { user: UserDTO; onDone: (updated: UserDTO) => void; onCancel: () => void };

const FIELDS = [
  { key: 'name', label: 'Имя' },
  { key: 'handle', label: 'Имя пользователя' },
  { key: 'city', label: 'Город' },
  { key: 'work', label: 'Работа' },
  { key: 'bio', label: 'О себе' },
] as const;

// Собираем ТОЛЬКО изменённые поля. Отправить всю форму нельзя: у бэкенда
// отсутствующее поле значит «не трогать», а пустая строка — «очистить»,
// поэтому полная форма затрёт всё, что пользователь не заполнил.
function changedOnly(draft: Record<string, string>, original: UserDTO): UpdateMePayload {
  const patch: Record<string, string> = {};
  for (const { key } of FIELDS) {
    if (draft[key] !== original[key]) patch[key] = draft[key];
  }
  return patch as UpdateMePayload;
}

export function EditProfileForm({ user, onDone, onCancel }: Props) {
  const [draft, setDraft] = useState<Record<string, string>>({
    name: user.name, handle: user.handle, city: user.city, work: user.work, bio: user.bio,
  });
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: (patch: UpdateMePayload) => usersApi.updateMe(patch),
    onMutate: () => tokenStore.epoch(),
    onSuccess: (updated, _patch, epoch) => { if (epoch === tokenStore.epoch()) onDone(updated); },
    onError: (err) =>
      setError(
        err instanceof ApiError && err.status === 409
          ? 'Это имя пользователя уже занято'
          : 'Не удалось сохранить',
      ),
  });

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);

    const patch = changedOnly(draft, user);
    // Ничего не менялось — не тревожим сервер вовсе.
    if (Object.keys(patch).length === 0) {
      onCancel();
      return;
    }
    save.mutate(patch);
  };

  return (
    <form className="card" onSubmit={submit} style={{ marginTop: 20 }}>
      {FIELDS.map(({ key, label }) => {
        const id = `profile-${key}`;
        return (
          <div className="field" key={key}>
            <label htmlFor={id}>{label}</label>
            {key === 'bio' ? (
              <textarea
                id={id}
                className="inp ta"
                value={draft[key]}
                rows={3}
                onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))}
              />
            ) : (
              <input
                id={id}
                className="inp"
                value={draft[key]}
                onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))}
              />
            )}
          </div>
        );
      })}

      {error && <div className="errorbox" role="alert" style={{ marginBottom: 14 }}>{error}</div>}

      <div style={{ display: 'flex', gap: 8 }}>
        <button className="btn btn-primary btn-sm" type="submit" disabled={save.isPending}>
          {save.isPending ? 'Сохраняем…' : 'Сохранить'}
        </button>
        <button className="btn btn-ghost btn-sm" type="button" onClick={onCancel}>
          Отмена
        </button>
      </div>
    </form>
  );
}
