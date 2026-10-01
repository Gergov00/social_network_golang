// Токен живёт в замыкании модуля и НИКОГДА не попадает в localStorage:
// оттуда его читает любой скрипт на странице. Переживать перезагрузку
// незачем — на старте приложение всё равно дёргает /auth/refresh по
// httpOnly-cookie, которую JS не видит в принципе.
let accessToken: string | null = null;
let epoch = 0;
const listeners = new Set<() => void>();

export const tokenStore = {
  epoch: () => epoch,
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
  },
  get(): string | null {
    return accessToken;
  },
  set(token: string): void {
    accessToken = token;
  },
  clear(): void {
    epoch += 1;
    accessToken = null;
    listeners.forEach((listener) => listener());
  },
};
