import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { expect, test } from 'vitest';
import { server } from '../test/server';
import { Providers } from '../app/providers';
import { AppRouter } from '../app/router';
import { tokenStore } from '../api/tokenStore';
import type { UserDTO } from '../api/types';

const user: UserDTO = {
  id: '2ccd9cf2-3c79-4b08-b453-236f5c3cd827',
  name: '',
  handle: 'u_2ccd9cf2',
  bio: '',
  city: '',
  work: '',
  verified: false,
  avatarUrl: null,
  createdAt: '2026-08-07T12:00:00Z',
  updatedAt: '2026-08-07T12:00:00Z',
};

const emptyFeed = () => http.get('/feed', () => HttpResponse.json({ posts: [], nextCursor: '' }));

function renderApp(path = '/') {
  window.history.pushState({}, '', path);
  return render(
    <Providers>
      <AppRouter />
    </Providers>,
  );
}

test('провал refresh на старте показывает вход, а не вечный спиннер', async () => {
  server.use(http.post('/auth/refresh', () => new HttpResponse(null, { status: 401 })));

  renderApp('/');

  expect(await screen.findByRole('heading', { name: /войти/i })).toBeInTheDocument();
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
});

test('аноним на защищённом маршруте попадает на /login', async () => {
  server.use(http.post('/auth/refresh', () => new HttpResponse(null, { status: 401 })));

  renderApp('/me');

  await screen.findByRole('heading', { name: /войти/i });
  expect(window.location.pathname).toBe('/login');
});

test('успешный refresh на старте пускает в ленту', async () => {
  server.use(
    http.post('/auth/refresh', () => HttpResponse.json({ access_token: 'tkn' })),
    http.get('/me', () => HttpResponse.json(user)),
    emptyFeed(),
  );

  renderApp('/');

  expect(await screen.findByPlaceholderText(/поделитесь мыслью/i)).toBeInTheDocument();
});

test('вход отправляет форму и не кладёт токен в localStorage', async () => {
  server.use(
    http.post('/auth/refresh', () => new HttpResponse(null, { status: 401 })),
    http.post('/auth/login', () => HttpResponse.json({ access_token: 'tkn' })),
    http.get('/me', () => HttpResponse.json(user)),
    emptyFeed(),
  );

  renderApp('/login');
  await screen.findByRole('heading', { name: /войти/i });

  await userEvent.type(screen.getByLabelText(/e-mail/i), 'feed@example.com');
  await userEvent.type(screen.getByLabelText(/пароль/i), 'password123');
  await userEvent.click(screen.getByRole('button', { name: /войти/i }));

  expect(await screen.findByPlaceholderText(/поделитесь мыслью/i)).toBeInTheDocument();
  expect(Object.keys(localStorage)).toHaveLength(0);
  expect(tokenStore.get()).toBe('tkn');
});

test('после регистрации /me ретраится, пока NATS не заведёт профиль', async () => {
  // Профиль создаётся АСИНХРОННО: auth публикует user.registered, а
  // user-service заводит профиль по событию. Поэтому GET /me сразу после
  // регистрации штатно отдаёт 404 — это наблюдалось вживую. Без ретрая
  // новый пользователь попадает в ленту с пустой шапкой.
  let meCalls = 0;
  server.use(
    http.post('/auth/refresh', () => new HttpResponse(null, { status: 401 })),
    http.post('/auth/register', () => HttpResponse.json({ access_token: 'tkn' }, { status: 201 })),
    http.get('/me', () => {
      meCalls += 1;
      return meCalls < 3 ? new HttpResponse(null, { status: 404 }) : HttpResponse.json(user);
    }),
    emptyFeed(),
  );

  renderApp('/register');
  await screen.findByRole('heading', { name: /создать аккаунт/i });

  await userEvent.type(screen.getByLabelText(/e-mail/i), 'new@example.com');
  await userEvent.type(screen.getByLabelText(/пароль/i), 'password123');
  await userEvent.click(screen.getByRole('button', { name: /создать аккаунт/i }));

  await waitFor(() => expect(meCalls).toBeGreaterThanOrEqual(3), { timeout: 5000 });
  expect(await screen.findByPlaceholderText(/поделитесь мыслью/i)).toBeInTheDocument();
});

test('занятый e-mail показывает сообщение, а не роняет форму', async () => {
  server.use(
    http.post('/auth/refresh', () => new HttpResponse(null, { status: 401 })),
    http.post('/auth/register', () =>
      HttpResponse.json({ error: 'Conflict' }, { status: 409 }),
    ),
  );

  renderApp('/register');
  await screen.findByRole('heading', { name: /создать аккаунт/i });

  await userEvent.type(screen.getByLabelText(/e-mail/i), 'taken@example.com');
  await userEvent.type(screen.getByLabelText(/пароль/i), 'password123');
  await userEvent.click(screen.getByRole('button', { name: /создать аккаунт/i }));

  expect(await screen.findByRole('alert')).toHaveTextContent(/занят|conflict/i);
});
