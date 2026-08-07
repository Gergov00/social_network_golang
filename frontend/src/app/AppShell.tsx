import { NavLink, Outlet } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { Avatar } from '../ui/Avatar';
import { Icon, LogoMark } from '../ui/Icons';
import { Topbar } from '../features/feed/Topbar';

// Разделы, которых бэкенд не умеет. Рисуем — иначе рейл выглядит обрубком, —
// но неактивными: интерфейс не должен обещать несуществующего.
const SOON = [
  { icon: 'comment', label: 'Сообщения' },
  { icon: 'user', label: 'Сообщества' },
  { icon: 'heart', label: 'Уведомления' },
] as const;

export function AppShell() {
  const { user } = useAuth();

  return (
    <div className="app">
      <nav className="rail" aria-label="Разделы">
        <div className="logo"><LogoMark size={26} /></div>
        <div className="rail-nav">
          <NavLink
            to="/"
            end
            className={({ isActive }) => `rail-btn${isActive ? ' active' : ''}`}
            aria-label="Лента"
            title="Лента"
          >
            <Icon name="feed" size={24} />
          </NavLink>

          {SOON.map(({ icon, label }) => (
            <button key={label} className="rail-btn" type="button" disabled title={`${label} — скоро`} aria-label={`${label} — скоро`}>
              <Icon name={icon} size={24} />
            </button>
          ))}
        </div>

        <span className="rail-spacer" />

        <NavLink to="/me" className="rail-ava" aria-label="Мой профиль" title="Мой профиль">
          <Avatar id={user?.id ?? ''} name={user?.name ?? ''} handle={user?.handle ?? ''} size={40} />
        </NavLink>
      </nav>

      <div className="main">
        <Topbar />
        <Outlet />
      </div>
    </div>
  );
}
