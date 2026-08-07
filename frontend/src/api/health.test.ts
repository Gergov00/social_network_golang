import { http, HttpResponse } from 'msw';
import { expect, test } from 'vitest';
import { server } from '../test/server';

// Тест стенда, а не приложения: убеждаемся, что MSW перехватывает запросы
// и что необъявленный запрос роняет тест (onUnhandledRequest: 'error').
test('MSW перехватывает запрос', async () => {
  server.use(http.get('/health', () => HttpResponse.json({ status: 'ok' })));

  const res = await fetch('/health');

  expect(res.status).toBe(200);
  expect(await res.json()).toEqual({ status: 'ok' });
});
