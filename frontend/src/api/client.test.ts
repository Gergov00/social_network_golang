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

test('повторный 401 завершает сессию после успешного refresh', async () => {
  const lost = vi.fn();
  setOnAuthLost(lost);
  server.use(
    http.get('/me', () => new HttpResponse(null, { status: 401 })),
    http.post('/auth/refresh', () => HttpResponse.json({ access_token: 'fresh' })),
  );
  await expect(apiFetch('/me')).rejects.toMatchObject({ status: 401 });
  expect(lost).toHaveBeenCalledTimes(1);
  expect(tokenStore.get()).toBeNull();
});

test('завершившийся после выхода refresh не возвращает токен', async () => {
  let release!: () => void;
  const pending = new Promise<void>((resolve) => { release = resolve; });
  let started = false;
  server.use(http.post('/auth/refresh', async () => {
    started = true;
    await pending;
    return HttpResponse.json({ access_token: 'obsolete' });
  }));
  const request = authApi.refresh().catch(() => {});
  while (!started) await new Promise((r) => setTimeout(r, 1));
  tokenStore.clear();
  release();
  await request;
  expect(tokenStore.get()).toBeNull();
});

test('старый 401 после смены аккаунта не refresh-ит новую сессию', async () => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  let started = false;
  let refreshCalls = 0;
  server.use(
    http.get('/feed', async () => { started = true; await gate; return new HttpResponse(null, { status: 401 }); }),
    http.post('/auth/refresh', () => { refreshCalls += 1; return HttpResponse.json({ access_token: 'obsolete' }); }),
  );
  const request = apiFetch('/feed').catch((error) => error);
  while (!started) await new Promise((resolve) => setTimeout(resolve, 1));
  tokenStore.clear();
  tokenStore.set('B');
  release();
  expect(await request).toMatchObject({ name: 'AbortError' });
  expect(refreshCalls).toBe(0);
  expect(tokenStore.get()).toBe('B');
});

test('старый ответ login после выхода не возвращает access token', async () => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  let started = false;
  server.use(http.post('/auth/login', async () => { started = true; await gate; return HttpResponse.json({ access_token: 'obsolete' }); }));
  const request = authApi.login('A', 'password').catch((error) => error);
  while (!started) await new Promise((resolve) => setTimeout(resolve, 1));
  tokenStore.clear();
  release();
  expect(await request).toMatchObject({ name: 'AbortError' });
  expect(tokenStore.get()).toBeNull();
});

test('новый login ждёт применения cookie старого refresh и logout', async () => {
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  const order: string[] = [];
  server.use(
    http.post('/auth/refresh', async () => { order.push('refresh-start'); await gate; order.push('refresh-cookie'); return HttpResponse.json({ access_token: 'A' }, { headers: { 'Set-Cookie': 'refresh_token=A; HttpOnly' } }); }),
    http.post('/auth/logout', () => { order.push('logout-cookie'); return new HttpResponse(null, { status: 204, headers: { 'Set-Cookie': 'refresh_token=; Max-Age=0' } }); }),
    http.post('/auth/login', () => { order.push('login-cookie-B'); return HttpResponse.json({ access_token: 'B' }, { headers: { 'Set-Cookie': 'refresh_token=B; HttpOnly' } }); }),
  );
  const oldRefresh = authApi.refresh().catch(() => {});
  while (!order.length) await new Promise((resolve) => setTimeout(resolve, 1));
  const logout = authApi.logout().catch(() => {});
  const login = authApi.login('B', 'password');
  await new Promise((resolve) => setTimeout(resolve, 30));
  const dispatchedBeforeRelease = [...order];
  release();
  await Promise.all([oldRefresh, logout, login]);
  expect(dispatchedBeforeRelease).toEqual(['refresh-start']);
  expect(order).toEqual(['refresh-start', 'refresh-cookie', 'logout-cookie', 'login-cookie-B']);
  expect(tokenStore.get()).toBe('B');
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
