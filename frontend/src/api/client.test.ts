import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { server } from '../test/server';
import { authApi } from './auth';
import { ApiError, apiFetch, setOnAuthLost } from './client';
import { tokenStore } from './tokenStore';

beforeEach(() => tokenStore.set('old-token'));
afterEach(() => {
  tokenStore.clear();
  setOnAuthLost(() => {});
});

test('подставляет Authorization из tokenStore', async () => {
  let seen: string | null = null;
  server.use(
    http.get('/me', ({ request }) => {
      seen = request.headers.get('Authorization');
      return HttpResponse.json({ id: '1' });
    }),
  );

  await apiFetch('/me');

  expect(seen).toBe('Bearer old-token');
});

describe('обновление токена', () => {
  test('два параллельных 401 дают РОВНО ОДИН POST /auth/refresh', async () => {
    // Это главный страж файла. Бэкенд делает reuse-detection: второй refresh
    // предъявит уже использованный токен, auth сочтёт это кражей и отзовёт
    // ВСЮ семью токенов (RevokeByFamilyID) — пользователя выкинет со всех
    // устройств. Поэтому одновременных refresh быть не должно ни одного
    // лишнего.
    let refreshCalls = 0;
    server.use(
      http.post('/auth/refresh', async () => {
        refreshCalls += 1;
        await new Promise((r) => setTimeout(r, 20));
        return HttpResponse.json({ access_token: 'fresh-token' });
      }),
      http.get('/me', ({ request }) =>
        request.headers.get('Authorization') === 'Bearer fresh-token'
          ? HttpResponse.json({ id: 'me' })
          : new HttpResponse(null, { status: 401 }),
      ),
      http.get('/feed', ({ request }) =>
        request.headers.get('Authorization') === 'Bearer fresh-token'
          ? HttpResponse.json({ posts: [], nextCursor: '' })
          : new HttpResponse(null, { status: 401 }),
      ),
    );

    const [me, feed] = await Promise.all([apiFetch('/me'), apiFetch('/feed')]);

    expect(refreshCalls).toBe(1);
    expect(me).toEqual({ id: 'me' });
    expect(feed).toEqual({ posts: [], nextCursor: '' });
    expect(tokenStore.get()).toBe('fresh-token');
  });

  test('authApi.refresh() параллельно с 401 в apiFetch даёт РОВНО ОДИН POST /auth/refresh', async () => {
    // Ровно шов bootstrap-а: восстановление сессии (authApi.refresh()) на
    // старте приложения идёт одновременно с загрузкой данных (apiFetch
    // ловит 401). Оба обязаны дождаться ОДНОГО общего промиса refreshOnce(),
    // а не запускать каждый свой /auth/refresh — иначе второй уходит с уже
    // использованным токеном и роняет всю семью на auth-service.
    let refreshCalls = 0;
    server.use(
      http.post('/auth/refresh', async () => {
        refreshCalls += 1;
        await new Promise((r) => setTimeout(r, 20));
        return HttpResponse.json({ access_token: 'fresh-token' });
      }),
      http.get('/me', ({ request }) =>
        request.headers.get('Authorization') === 'Bearer fresh-token'
          ? HttpResponse.json({ id: 'me' })
          : new HttpResponse(null, { status: 401 }),
      ),
    );

    const [me] = await Promise.all([apiFetch('/me'), authApi.refresh()]);

    expect(refreshCalls).toBe(1);
    expect(me).toEqual({ id: 'me' });
    expect(tokenStore.get()).toBe('fresh-token');
  });

  test('повторяет исходный запрос с новым токеном', async () => {
    const seen: string[] = [];
    server.use(
      http.post('/auth/refresh', () => HttpResponse.json({ access_token: 'fresh-token' })),
      http.get('/me', ({ request }) => {
        const auth = request.headers.get('Authorization') ?? '';
        seen.push(auth);
        return auth === 'Bearer fresh-token'
          ? HttpResponse.json({ id: 'me' })
          : new HttpResponse(null, { status: 401 });
      }),
    );

    await apiFetch('/me');

    expect(seen).toEqual(['Bearer old-token', 'Bearer fresh-token']);
  });

  test('провал refresh чистит токен и зовёт onAuthLost', async () => {
    const onAuthLost = vi.fn();
    setOnAuthLost(onAuthLost);
    server.use(
      http.post('/auth/refresh', () => new HttpResponse(null, { status: 401 })),
      http.get('/me', () => new HttpResponse(null, { status: 401 })),
    );

    await expect(apiFetch('/me')).rejects.toBeInstanceOf(ApiError);

    expect(tokenStore.get()).toBeNull();
    expect(onAuthLost).toHaveBeenCalledTimes(1);
  });

  test('не пытается обновляться на самом /auth/refresh', async () => {
    let refreshCalls = 0;
    server.use(
      http.post('/auth/refresh', () => {
        refreshCalls += 1;
        return new HttpResponse(null, { status: 401 });
      }),
    );

    await expect(apiFetch('/auth/refresh', { method: 'POST', skipAuth: true })).rejects.toBeInstanceOf(
      ApiError,
    );

    // Ровно один: иначе неудачный refresh рекурсивно вызывал бы сам себя.
    expect(refreshCalls).toBe(1);
  });
});

test('ApiError несёт статус и текст из поля error', async () => {
  server.use(
    http.get('/posts/x', () => HttpResponse.json({ error: 'Not Found' }, { status: 404 })),
  );

  const err = await apiFetch('/posts/x').catch((e) => e);

  expect(err).toBeInstanceOf(ApiError);
  expect((err as ApiError).status).toBe(404);
  expect((err as ApiError).message).toBe('Not Found');
});

test('204 не пытается разобрать пустое тело как JSON', async () => {
  server.use(http.post('/auth/logout', () => new HttpResponse(null, { status: 204 })));

  await expect(apiFetch('/auth/logout', { method: 'POST' })).resolves.toBeNull();
});

test('токен не утекает в localStorage', async () => {
  server.use(http.get('/me', () => HttpResponse.json({ id: 'me' })));

  await apiFetch('/me');

  expect(localStorage.getItem('access_token')).toBeNull();
  expect(Object.keys(localStorage)).toHaveLength(0);
});
