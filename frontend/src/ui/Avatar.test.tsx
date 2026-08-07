import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { Avatar } from './Avatar';

test('берёт инициалы из имени', () => {
  render(<Avatar id="a" name="Аня Соколова" handle="u_anya" />);
  expect(screen.getByText('АС')).toBeInTheDocument();
});

// Бэкенд заводит профиль со стаб-хендлом и ПУСТЫМ именем (см. user-service:
// CreateProfile ставит Name=""). Если брать инициалы только из name, у всех
// новых пользователей аватар будет пустым кружком.
test('при пустом имени берёт инициалы из handle', () => {
  render(<Avatar id="a" name="" handle="u_2ccd9cf2" />);
  expect(screen.getByText('U2')).toBeInTheDocument();
});

test('градиент детерминирован по id, а не случаен', () => {
  const { container: first } = render(<Avatar id="same-id" name="Кто-то" handle="h" />);
  const { container: second } = render(<Avatar id="same-id" name="Другой Человек" handle="h2" />);

  const bg = (el: Element) => (el.firstChild as HTMLElement).style.background;
  expect(bg(first)).toBe(bg(second));
  expect(bg(first)).not.toBe('');
});
