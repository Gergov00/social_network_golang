import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState } from 'react';
import { BrowserRouter } from 'react-router-dom';
import type { ReactNode } from 'react';
import { AuthProvider } from '../auth/AuthContext';
import { ApiError } from '../api/client';

export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // Ретраить 4xx бессмысленно: 401 уже разрулил apiFetch, а 404 от
        // повтора не изменится. Ретраим только сетевые и 5xx, один раз.
        retry: (count, error) =>
          count < 1 && !(error instanceof ApiError && error.status < 500),
        refetchOnWindowFocus: false,
      },
    },
  });
}

export function Providers({ children }: { children: ReactNode }) {
  // useState(createQueryClient) — ленивый инициализатор: клиент создаётся
  // ровно один раз при монтировании. Прямой вызов createQueryClient() в JSX
  // пересоздавал бы его на каждый рендер Providers, молча обнуляя весь кэш
  // при любом будущем состоянии/ре-рендере этого компонента.
  const [client] = useState(createQueryClient);
  return (
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <AuthProvider>{children}</AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
