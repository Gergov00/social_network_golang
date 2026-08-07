# ahishka — frontend

SPA на Vite + React + TypeScript поверх gateway.

## Запуск

Сначала бэкенд (из корня репозитория):

```
docker compose up -d
```

Затем фронт:

```
npm install
npm run dev
```

Открыть http://localhost:5173

## Как устроены запросы

Dev-сервер проксирует **реальные пути** (`/auth`, `/me`, `/users`, `/feed`, `/posts`)
на `http://localhost:8080`. Префикс `/api` не используется намеренно: refresh-cookie
выставлена с `Path=/auth` и на `/api/auth/refresh` браузером не отправится.

Поскольку запросы идут с того же origin, CORS не нужен — и в gateway его нет.

Access-токен хранится только в памяти. При загрузке страницы приложение делает
`POST /auth/refresh` по httpOnly-cookie. Обновление токена — **single-flight**:
параллельные refresh вызвали бы на бэкенде reuse-detection и отзыв всей семьи токенов.

## Тесты

```
npm test
```

Vitest + Testing Library + MSW. Покрыто поведение, а не вёрстка.
