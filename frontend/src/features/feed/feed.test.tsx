import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { delay, http, HttpResponse } from 'msw';
import { expect, test } from 'vitest';
import { QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import { server } from '../../test/server';
import { Providers, createQueryClient } from '../../app/providers';
import { AppRouter } from '../../app/router';
import { AuthProvider } from '../../auth/AuthContext';
import type { PostDTO, UserDTO } from '../../api/types';

const author: UserDTO = {
  id: 'author-1', name: 'Аня Соколова', handle: 'anya', bio: '', city: '', work: '',
  verified: true, avatarUrl: null,
  createdAt: '2026-08-07T10:00:00Z', updatedAt: '2026-08-07T10:00:00Z',
};

const me: UserDTO = { ...author, id: 'me-1', name: '', handle: 'u_me', verified: false };

function post(over: Partial<PostDTO> = {}): PostDTO {
  return {
    id: 'p1', authorId: author.id, author, text: 'первый пост',
    likes: 3, liked: false, comments: 0,
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

function renderFeed() {
  window.history.pushState({}, '', '/');
  return render(
    <Providers>
      <AppRouter />
    </Providers>,
  );
}

// Как renderFeed(), но с явным QueryClient — чтобы тест мог заглянуть в кэш
// напрямую через client.getQueryData/setQueryData, минуя реальный экран
// поста (которого пока нет — его добавит следующая задача).
function renderFeedWithClient() {
  window.history.pushState({}, '', '/');
  const client = createQueryClient();
  render(
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <AuthProvider>
          <AppRouter />
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>,
  );
  return client;
}

test('пустая лента показывает пустое состояние, а не пустой экран', async () => {
  server.use(...authed(), http.get('/feed', () => HttpResponse.json({ posts: [], nextCursor: '' })));

  renderFeed();

  expect(await screen.findByText(/пока пусто|записей нет/i)).toBeInTheDocument();
});

test('рисует пост с автором', async () => {
  server.use(
    ...authed(),
    http.get('/feed', () => HttpResponse.json({ posts: [post()], nextCursor: '' })),
  );

  renderFeed();

  expect(await screen.findByText('первый пост')).toBeInTheDocument();
  expect(screen.getByText('Аня Соколова')).toBeInTheDocument();
});

test('пост без автора отрисован, а не пропущен', async () => {
  // gateway отдаёт author: null, когда профиль не найден или user-service
  // не ответил. Это деградация, а не ошибка: прятать такой пост нельзя.
  server.use(
    ...authed(),
    http.get('/feed', () =>
      HttpResponse.json({ posts: [post({ author: null, text: 'осиротевший' })], nextCursor: '' }),
    ),
  );

  renderFeed();

  expect(await screen.findByText('осиротевший')).toBeInTheDocument();
});

test('лайк оптимистичен и откатывается при ошибке сервера', async () => {
  server.use(
    ...authed(),
    http.get('/feed', () => HttpResponse.json({ posts: [post({ likes: 3, liked: false })], nextCursor: '' })),
    // Искусственная задержка ответа: без неё синхронный onMutate и
    // последующий откат в onError укладываются в один и тот же микротаск,
    // и «4» никогда не долетает до DOM как отдельный наблюдаемый кадр —
    // тест видел бы «3» → «3» и на бракованном коде тоже (см. пункт 3
    // финального ревью). Задержка даёт waitFor реальное окно, где в кэше
    // лежит именно оптимистичное значение.
    http.post('/posts/p1/like', async () => {
      await delay(50);
      return HttpResponse.json({ error: 'Internal Server Error' }, { status: 500 });
    }),
  );

  renderFeed();
  const likeButton = await screen.findByRole('button', { name: /нравится/i });
  expect(likeButton).toHaveTextContent('3');

  await userEvent.click(likeButton);

  // Сначала оптимистично 4 — ДО ответа сервера. Без этого промежуточного
  // ассерта тест не отличает «онМutate вообще не патчил кэш» от «патчил и
  // откатил»: оба случая заканчиваются одинаковым «3» в конце.
  await waitFor(() => expect(likeButton).toHaveTextContent('4'));

  // ...затем откат к 3, когда сервер ответил 500.
  await waitFor(() => expect(likeButton).toHaveTextContent('3'));
});

test('успешный лайк берёт счётчик из ответа сервера', async () => {
  server.use(
    ...authed(),
    http.get('/feed', () => HttpResponse.json({ posts: [post({ likes: 3, liked: false })], nextCursor: '' })),
    // Сервер знает правду: у него 41, а не 4 — клиент обязан взять его число.
    http.post('/posts/p1/like', () => HttpResponse.json(post({ likes: 41, liked: true }))),
  );

  renderFeed();
  const likeButton = await screen.findByRole('button', { name: /нравится/i });

  await userEvent.click(likeButton);

  await waitFor(() => expect(likeButton).toHaveTextContent('41'));
});

test('при nextCursor="" догрузка не запускается', async () => {
  // Пустая строка означает конец ленты. Если её вернуть как pageParam,
  // TanStack Query решит, что страница есть, и будет грузить бесконечно.
  let feedCalls = 0;
  server.use(
    ...authed(),
    http.get('/feed', () => {
      feedCalls += 1;
      return HttpResponse.json({ posts: [post()], nextCursor: '' });
    }),
  );

  renderFeed();
  await screen.findByText('первый пост');

  await new Promise((r) => setTimeout(r, 50));
  expect(feedCalls).toBe(1);
  expect(screen.queryByRole('button', { name: /ещё|загрузить/i })).not.toBeInTheDocument();
});

test('вторая страница запрашивается с курсором первой', async () => {
  const seen: (string | null)[] = [];
  server.use(
    ...authed(),
    http.get('/feed', ({ request }) => {
      const cursor = new URL(request.url).searchParams.get('cursor');
      seen.push(cursor);
      return cursor
        ? HttpResponse.json({ posts: [post({ id: 'p2', text: 'второй пост' })], nextCursor: '' })
        : HttpResponse.json({ posts: [post()], nextCursor: 'CURSOR-1' });
    }),
  );

  renderFeed();
  await screen.findByText('первый пост');

  await userEvent.click(await screen.findByRole('button', { name: /ещё/i }));

  expect(await screen.findByText('второй пост')).toBeInTheDocument();
  expect(seen).toEqual([null, 'CURSOR-1']);
});

test('опубликованный пост встаёт в начало ленты', async () => {
  server.use(
    ...authed(),
    http.get('/feed', () => HttpResponse.json({ posts: [post()], nextCursor: '' })),
    http.post('/posts', () =>
      HttpResponse.json(post({ id: 'p-new', text: 'свежая мысль' }), { status: 201 }),
    ),
  );

  renderFeed();
  await userEvent.click(await screen.findByPlaceholderText(/поделитесь мыслью/i));
  await userEvent.type(screen.getByRole('textbox'), 'свежая мысль');
  await userEvent.click(screen.getByRole('button', { name: /опубликовать/i }));

  const posts = await screen.findAllByRole('article');
  expect(within(posts[0]).getByText('свежая мысль')).toBeInTheDocument();
});

test('ошибка публикации показана, а текст поста не пропадает', async () => {
  server.use(
    ...authed(),
    http.get('/feed', () => HttpResponse.json({ posts: [post()], nextCursor: '' })),
    http.post('/posts', () => HttpResponse.json({ error: 'Internal Server Error' }, { status: 500 })),
  );

  renderFeed();
  await userEvent.click(await screen.findByPlaceholderText(/поделитесь мыслью/i));
  await userEvent.type(screen.getByRole('textbox'), 'свежая мысль');
  await userEvent.click(screen.getByRole('button', { name: /опубликовать/i }));

  expect(await screen.findByRole('alert')).toBeInTheDocument();
  // Текст не пропадает — пользователь может просто повторить попытку.
  expect(screen.getByRole('textbox')).toHaveValue('свежая мысль');
});

test('нереализованные действия видны, но неактивны', async () => {
  server.use(
    ...authed(),
    http.get('/feed', () => HttpResponse.json({ posts: [post()], nextCursor: '' })),
  );

  renderFeed();
  await screen.findByText('первый пост');

  // Комментарии теперь работают: это ссылка на страницу поста со счётчиком,
  // а не заглушка. Репост и закладки бэкенд по-прежнему не умеет.
  expect(screen.getByRole('link', { name: /комментар/i })).toHaveAttribute('href', '/post/p1');
  expect(screen.getByRole('button', { name: /репост/i })).toBeDisabled();
  expect(screen.getByRole('button', { name: /закладк/i })).toBeDisabled();
});

test('откат лайка при ошибке восстанавливает и ленту, и кэш страницы поста', async () => {
  // Страницы поста ещё нет (её добавит следующая задача), но её кэш —
  // ['post', id] — уже патчится оптимистично в useLike.onMutate. Если
  // onError откатывает только ленту, эта запись останется оптимистичной
  // навсегда — ровно та регрессия, которую эта задача обязана перекрыть.
  server.use(
    ...authed(),
    http.get('/feed', () => HttpResponse.json({ posts: [post({ likes: 3, liked: false })], nextCursor: '' })),
    http.post('/posts/p1/like', () => HttpResponse.json({ error: 'Internal Server Error' }, { status: 500 })),
  );

  const client = renderFeedWithClient();
  // Кэш страницы поста уже существует к моменту лайка (например, пользователь
  // недавно её открывал) — так onMutate будет что патчить, а не no-op на
  // отсутствующей записи.
  client.setQueryData<PostDTO>(['post', 'p1'], post({ likes: 3, liked: false }));

  const likeButton = await screen.findByRole('button', { name: /нравится/i });
  await userEvent.click(likeButton);

  await waitFor(() => expect(likeButton).toHaveTextContent('3'));
  expect(client.getQueryData<PostDTO>(['post', 'p1'])).toMatchObject({ likes: 3, liked: false });
});
