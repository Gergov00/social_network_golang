import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { delay, http, HttpResponse } from 'msw';
import { expect, test } from 'vitest';
import { server } from '../../test/server';
import { Providers } from '../../app/providers';
import { AppRouter } from '../../app/router';
import type { PostDTO, UserDTO } from '../../api/types';

const me: UserDTO = {
  id: 'me-1', name: 'Аня Соколова', handle: 'anya',
  bio: 'делаю интерфейсы', city: 'Казань', work: 'дизайнер',
  verified: false, avatarUrl: null,
  createdAt: '2026-08-07T10:00:00Z', updatedAt: '2026-08-07T10:00:00Z',
};

// Отдельный от `me` пользователь для тестов чужого профиля: `me.handle` уже
// занят 'anya', поэтому здесь — 'boris', не 'anya' из брифа задачи.
const author: UserDTO = {
  id: 'boris-1', name: 'Борис Орлов', handle: 'boris',
  bio: '', city: '', work: '',
  verified: true, avatarUrl: null,
  createdAt: '2026-08-07T10:00:00Z', updatedAt: '2026-08-07T10:00:00Z',
};

function post(over: Partial<PostDTO> = {}): PostDTO {
  return {
    id: 'p1', authorId: author.id, author, text: 'первый пост',
    likes: 3, liked: false,
    createdAt: '2026-08-07T12:00:00Z', updatedAt: '2026-08-07T12:00:00Z',
    ...over,
  };
}

function authed() {
  return [
    http.post('/auth/refresh', () => HttpResponse.json({ access_token: 'tkn' })),
    http.get('/me', () => HttpResponse.json(me)),
  ];
}

function renderAt(path: string) {
  window.history.pushState({}, '', path);
  return render(
    <Providers>
      <AppRouter />
    </Providers>,
  );
}

test('свой профиль показывает данные из /me', async () => {
  server.use(...authed());

  renderAt('/me');

  expect(await screen.findByText('Аня Соколова')).toBeInTheDocument();
  expect(screen.getByText(/@anya/)).toBeInTheDocument();
  expect(screen.getByText('делаю интерфейсы')).toBeInTheDocument();
});

test('PATCH /me отправляет ТОЛЬКО изменённые поля', async () => {
  // Сквозная PATCH-семантика: отсутствующее поле значит «не трогать», а
  // пустая строка — «очистить». Форма, отправляющая все пять полей, затрёт
  // незаполненные — поэтому в теле обязано быть ровно одно поле.
  let body: unknown = null;
  server.use(
    ...authed(),
    http.patch('/me', async ({ request }) => {
      body = await request.json();
      return HttpResponse.json({ ...me, city: 'Уфа' });
    }),
  );

  renderAt('/me');
  await userEvent.click(await screen.findByRole('button', { name: /редактировать/i }));

  const city = screen.getByLabelText(/город/i);
  await userEvent.clear(city);
  await userEvent.type(city, 'Уфа');
  await userEvent.click(screen.getByRole('button', { name: /сохранить/i }));

  await waitFor(() => expect(body).toEqual({ city: 'Уфа' }));
});

test('форма без изменений не шлёт запрос вовсе', async () => {
  let calls = 0;
  server.use(
    ...authed(),
    http.patch('/me', () => {
      calls += 1;
      return HttpResponse.json(me);
    }),
  );

  renderAt('/me');
  await userEvent.click(await screen.findByRole('button', { name: /редактировать/i }));
  await userEvent.click(screen.getByRole('button', { name: /сохранить/i }));

  await new Promise((r) => setTimeout(r, 50));
  expect(calls).toBe(0);
});

test('занятый handle показывает ошибку у поля', async () => {
  server.use(
    ...authed(),
    http.patch('/me', () => HttpResponse.json({ error: 'Conflict' }, { status: 409 })),
  );

  renderAt('/me');
  await userEvent.click(await screen.findByRole('button', { name: /редактировать/i }));

  const handle = screen.getByLabelText(/имя пользователя/i);
  await userEvent.clear(handle);
  await userEvent.type(handle, 'taken');
  await userEvent.click(screen.getByRole('button', { name: /сохранить/i }));

  expect(await screen.findByRole('alert')).toHaveTextContent(/занят/i);
});

test('чужой профиль грузится по handle и не даёт редактировать', async () => {
  server.use(
    ...authed(),
    http.get('/users/tim', () => HttpResponse.json({ ...me, id: 'tim-1', name: 'Тимур', handle: 'tim' })),
  );

  renderAt('/u/tim');

  expect(await screen.findByText('Тимур')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /редактировать/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: /подписаться/i })).toBeDisabled();
});

test('несуществующий handle даёт понятное сообщение, а не пустой экран', async () => {
  server.use(
    ...authed(),
    http.get('/users/nobody', () => HttpResponse.json({ error: 'Not Found' }, { status: 404 })),
  );

  renderAt('/u/nobody');

  expect(await screen.findByText(/не найден/i)).toBeInTheDocument();
});

test('счётчики, которых бэкенд не знает, показаны как —', async () => {
  server.use(...authed());

  renderAt('/me');
  await screen.findByText('Аня Соколова');

  // Ноль был бы утверждением, и оно ложное: бэкенд этих чисел не отдаёт.
  expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(3);
});

test('страница записи показывает пост', async () => {
  server.use(
    ...authed(),
    http.get('/posts/p1', () =>
      HttpResponse.json({
        id: 'p1', authorId: me.id, author: me, text: 'запись целиком',
        likes: 2, liked: false,
        createdAt: '2026-08-07T12:00:00Z', updatedAt: '2026-08-07T12:00:00Z',
      }),
    ),
  );

  renderAt('/post/p1');

  expect(await screen.findByText('запись целиком')).toBeInTheDocument();
});

test('вкладка «Записи» на своём профиле грузит /me/posts', async () => {
  server.use(
    ...authed(),
    http.get('/me/posts', () =>
      HttpResponse.json({ posts: [post({ text: 'моя запись' })], nextCursor: '' }),
    ),
  );

  renderAt('/me');

  expect(await screen.findByText('моя запись')).toBeInTheDocument();
});

test('вкладка «Записи» на чужом профиле грузит /users/:handle/posts', async () => {
  // Ходить на /me/posts с чужого профиля — самая вероятная ошибка тут:
  // экран выглядел бы рабочим и показывал бы СВОИ записи под чужим именем.
  server.use(
    ...authed(),
    http.get('/users/boris', () => HttpResponse.json(author)),
    http.get('/users/boris/posts', () =>
      HttpResponse.json({ posts: [post({ text: 'запись Бориса' })], nextCursor: '' }),
    ),
    http.get('/me/posts', () =>
      HttpResponse.json({ posts: [post({ text: 'мои записи тут не место' })], nextCursor: '' }),
    ),
  );

  renderAt('/u/boris');

  expect(await screen.findByText('запись Бориса')).toBeInTheDocument();
  expect(screen.queryByText('мои записи тут не место')).not.toBeInTheDocument();
});

test('пустой список записей показывает пустое состояние, а не спиннер навсегда', async () => {
  server.use(
    ...authed(),
    http.get('/me/posts', () => HttpResponse.json({ posts: [], nextCursor: '' })),
  );

  renderAt('/me');

  expect(await screen.findByText(/записей пока нет|пока пусто/i)).toBeInTheDocument();
});

test('лайк во вкладке «Записи» откатывается при ошибке сервера', async () => {
  // Регрессия по конструкции: useLike патчил только кэш ленты (FEED_KEY).
  // Список записей профиля живёт под другим ключом — без правки useLike
  // счётчик тут не двигался бы вовсе, а после правки без отката — залипал бы.
  server.use(
    ...authed(),
    http.get('/me/posts', () =>
      HttpResponse.json({ posts: [post({ likes: 3, liked: false })], nextCursor: '' }),
    ),
    http.post('/posts/p1/like', async () => {
      await delay(50);
      return HttpResponse.json({ error: 'Internal Server Error' }, { status: 500 });
    }),
  );

  renderAt('/me');

  const likeButton = await screen.findByRole('button', { name: /нравится/i });
  expect(likeButton).toHaveTextContent('3');

  await userEvent.click(likeButton);
  await waitFor(() => expect(likeButton).toHaveTextContent('4'));
  await waitFor(() => expect(likeButton).toHaveTextContent('3'));
});
