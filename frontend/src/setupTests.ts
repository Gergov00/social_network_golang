import '@testing-library/jest-dom/vitest';
import { afterAll, afterEach, beforeAll, vi } from 'vitest';
import { cleanup } from '@testing-library/react';
import { server } from './test/server';

// onUnhandledRequest: 'error' — если код полез в сеть мимо объявленных
// хендлеров, тест обязан упасть, а не молча получить undefined.
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => {
  server.resetHandlers();
  cleanup();
  localStorage.clear();
});
afterAll(() => server.close());

// jsdom не реализует IntersectionObserver, а на нём держится догрузка ленты.
// Заглушка ничего не наблюдает: тесты пагинации дёргают загрузку напрямую.
class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
  root = null;
  rootMargin = '';
  thresholds = [];
}
vi.stubGlobal('IntersectionObserver', IntersectionObserverStub);
