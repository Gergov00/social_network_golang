// defineConfig берём из vitest/config, а не из vite: начиная с Vitest 3
// секцию `test` знает только он, и с `vite`-версией сборка падает на
// TS2769 («'test' does not exist in type 'UserConfigExport'»).
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Проксируем РЕАЛЬНЫЕ пути, а не префикс /api. Причина не в красоте:
// refresh-cookie выставлена бэкендом с Path=/auth, и на /api/auth/refresh
// браузер её просто не отправит — автологин молча сломается. Заодно dev
// один в один повторяет прод, где статика раздаётся с того же origin.
const apiPaths = ['/auth', '/me', '/users', '/feed', '/posts', '/comments', '/health'];

export default defineConfig({
  plugins: [react()],
  server: {
    // Явный IPv4: по умолчанию Vite слушает только [::1], и любой клиент,
    // резолвящий localhost в 127.0.0.1 (curl.exe, PowerShell, часть скриптов
    // проверки), получает «connection refused» на пустом месте.
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: Object.fromEntries(
      apiPaths.map((path) => [path, { target: 'http://localhost:8080', changeOrigin: false }]),
    ),
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/setupTests.ts',
    css: false,
  },
});
