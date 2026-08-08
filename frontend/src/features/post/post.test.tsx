import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { delay, http, HttpResponse } from 'msw';
import { expect, test } from 'vitest';
import { server } from '../../test/server';
import { Providers } from '../../app/providers';
import { AppRouter } from '../../app/router';
import type { CommentDTO, PostDTO, UserDTO } from '../../api/types';

const author: UserDTO = {
  id: 'author-1', name: 'Аня Соколова', handle: 'anya', bio: '', city: '', work: '',
  verified: true, avatarUrl: null,
  createdAt: '2026-08-08T10:00:00Z', updatedAt: '2026-08-08T10:00:00Z',
};

const me: UserDTO = { ...author, id: 'me-1', name: '', handle: 'u_me', verified: false };

function post(over: Partial<PostDTO> = {}): PostDTO {
  return {
    id: 'p1', authorId: author.id, author, text: 'пост с обсуждением',
    likes: 3, liked: false, comments: 1,
    createdAt: '2026-08-08T12:00:00Z', updatedAt: '2026-08-08T12:00:00Z',
    ...over,
  };
}

function comment(over: Partial<CommentDTO> = {}): CommentDTO {
  return {
    id: 'c1', postId: 'p1', authorId: author.id, author, text: 'первый комментарий',
    likes: 2, liked: false,
    createdAt: '2026-08-08T12:05:00Z', updatedAt: '2026-08-08T12:05:00Z',
    ...over,
  };
}

function authed() {
  return [
    http.post('/auth/refresh', () => HttpResponse.json({ access_token: 'tkn' })),
    http.get('/me', () => HttpResponse.json(me)),
    http.get('/posts/p1', () => HttpResponse.json(post())),
  ];
}

function renderPost() {
  window.history.pushState({}, '', '/post/p1');
  return render(<Providers><AppRouter /></Providers>);
}

test('показывает ветку комментариев', async () => {
  server.use(
    ...authed(),
    http.get('/posts/p1/comments', () =>
      HttpResponse.json({ comments: [comment()], nextCursor: '' }),
    ),
  );

  renderPost();

  expect(await screen.findByText('первый комментарий')).toBeInTheDocument();
});

test('комментарий без автора отрисован, а не пропущен', async () => {
  // gateway отдаёт author: null, когда профиль не найден или user-service
  // не ответил. Это деградация, а не ошибка: прятать реплику нельзя.
  server.use(
    ...authed(),
    http.get('/posts/p1/comments', () =>
      HttpResponse.json({ comments: [comment({ author: null, text: 'осиротевшая реплика' })], nextCursor: '' }),
    ),
  );

  renderPost();

  expect(await screen.findByText('осиротевшая реплика')).toBeInTheDocument();
});

test('пустая ветка показывает пустое состояние', async () => {
  server.use(
    ...authed(),
    http.get('/posts/p1/comments', () => HttpResponse.json({ comments: [], nextCursor: '' })),
  );

  renderPost();

  expect(await screen.findByText(/комментариев пока нет/i)).toBeInTheDocument();
});

test('отправленный комментарий появляется в ветке', async () => {
  server.use(
    ...authed(),
    http.get('/posts/p1/comments', () => HttpResponse.json({ comments: [], nextCursor: '' })),
    http.post('/posts/p1/comments', () =>
      HttpResponse.json(comment({ id: 'c-new', text: 'моя реплика' }), { status: 201 }),
    ),
  );

  renderPost();

  await userEvent.type(await screen.findByPlaceholderText(/комментар/i), 'моя реплика');
  await userEvent.click(screen.getByRole('button', { name: /отправить/i }));

  expect(await screen.findByText('моя реплика')).toBeInTheDocument();
});

test('ошибка отправки показана, а текст не пропадает', async () => {
  server.use(
    ...authed(),
    http.get('/posts/p1/comments', () => HttpResponse.json({ comments: [], nextCursor: '' })),
    http.post('/posts/p1/comments', () =>
      HttpResponse.json({ error: 'Internal Server Error' }, { status: 500 }),
    ),
  );

  renderPost();

  const field = await screen.findByPlaceholderText(/комментар/i);
  await userEvent.type(field, 'моя реплика');
  await userEvent.click(screen.getByRole('button', { name: /отправить/i }));

  expect(await screen.findByRole('alert')).toBeInTheDocument();
  expect(field).toHaveValue('моя реплика');
});

test('лайк комментария оптимистичен и откатывается при ошибке', async () => {
  server.use(
    ...authed(),
    http.get('/posts/p1/comments', () =>
      HttpResponse.json({ comments: [comment({ likes: 2, liked: false })], nextCursor: '' }),
    ),
    http.post('/comments/c1/like', async () => {
      await delay(50);
      return HttpResponse.json({ error: 'Internal Server Error' }, { status: 500 });
    }),
  );

  renderPost();

  const button = await screen.findByRole('button', { name: /нравится комментарий/i });
  expect(button).toHaveTextContent('2');

  await userEvent.click(button);
  await waitFor(() => expect(button).toHaveTextContent('3'));
  await waitFor(() => expect(button).toHaveTextContent('2'));
});
