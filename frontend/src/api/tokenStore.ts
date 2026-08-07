// Токен живёт в замыкании модуля и НИКОГДА не попадает в localStorage:
// оттуда его читает любой скрипт на странице. Переживать перезагрузку
// незачем — на старте приложение всё равно дёргает /auth/refresh по
// httpOnly-cookie, которую JS не видит в принципе.
let accessToken: string | null = null;

export const tokenStore = {
  get(): string | null {
    return accessToken;
  },
  set(token: string): void {
    accessToken = token;
  },
  clear(): void {
    accessToken = null;
  },
};
