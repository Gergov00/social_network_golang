import { Link } from 'react-router-dom';
import { useAuth } from '../../auth/AuthContext';
import { Icon } from '../../ui/Icons';

export function Topbar() {
  const { logout } = useAuth();

  return (
    <div className="topbar">
      <div className="topbar-in">
        <Link to="/" className="wordmark" style={{ textDecoration: 'none', color: 'inherit' }}>
          <span className="brand-a">a</span>hishka
        </Link>
        {/* Поиска на бэкенде нет — поле на месте, но неактивно.
            type="search" (роль searchbox), а не обычный текстовый input:
            иначе с открытым композером в DOM оказываются два элемента
            с ролью textbox и getByRole('textbox') в тестах ленты падает
            на неоднозначности. */}
        <label className="search">
          <Icon name="search" />
          <input type="search" placeholder="Поиск людей, групп, записей" disabled title="Скоро" />
        </label>
        {/* Аватар профиля живёт в рейле (.rail-ava), здесь его не дублируем. */}
        <span className="grow" />
        <button className="iconbtn" type="button" aria-label="Выйти" onClick={() => void logout()}>
          <Icon name="logout" />
        </button>
      </div>
    </div>
  );
}
