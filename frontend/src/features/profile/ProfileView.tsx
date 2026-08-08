import { useState } from 'react';
import type { UserDTO } from '../../api/types';
import { Avatar } from '../../ui/Avatar';
import { Empty } from '../../ui/Empty';
import { Icon } from '../../ui/Icons';
import { UserPosts } from './UserPosts';

type Props = { user: UserDTO; isMe: boolean; onEdit?: () => void };

// Бэкенд не отдаёт ни числа записей/друзей/медиа, ни списки для вкладок.
// Показываем «—», а не 0: ноль — это утверждение, и оно ложное.
const UNKNOWN = '—';

export function ProfileView({ user, isMe, onEdit }: Props) {
  const [tab, setTab] = useState<'posts' | 'media' | 'about'>('posts');
  const name = user.name.trim() || user.handle;

  return (
    <>
      <div className="cover" />
      <div className="profile-head">
        <Avatar id={user.id} name={user.name} handle={user.handle} size={118} />
        <h1 style={{ margin: '12px 0 2px', display: 'flex', alignItems: 'center', gap: 6 }}>
          {name}
          {user.verified && <span className="badge-v"><Icon name="check" size={18} /></span>}
        </h1>
        <div className="post-sub">
          @{user.handle}
          {user.city && ` · ${user.city}`}
          {user.work && ` · ${user.work}`}
        </div>
        {user.bio && <p style={{ marginTop: 12 }}>{user.bio}</p>}

        {/* .stat-row > .stat > b + span — как в макете. */}
        <div className="stat-row">
          <div className="stat"><b>{UNKNOWN}</b><span>записей</span></div>
          <div className="stat"><b>{UNKNOWN}</b><span>друзей</span></div>
          <div className="stat"><b>{UNKNOWN}</b><span>медиа</span></div>
        </div>

        {isMe ? (
          <button className="btn btn-primary btn-sm" type="button" onClick={onEdit}>
            Редактировать
          </button>
        ) : (
          <>
            {/* Графа подписок и сообщений на бэкенде нет. */}
            <button className="btn btn-primary btn-sm" type="button" disabled title="Скоро">
              Подписаться
            </button>
            <button className="btn btn-ghost btn-sm" type="button" disabled title="Скоро" style={{ marginLeft: 8 }}>
              Написать
            </button>
          </>
        )}

        {/* .tabs.plain > .tab-txt — подчёркивание активной вкладки рисует
            псевдоэлемент ::after, отдельной разметки не нужно. */}
        <div className="tabs plain mt16">
          <button type="button" className={`tab-txt${tab === 'posts' ? ' active' : ''}`} onClick={() => setTab('posts')}>Записи</button>
          <button type="button" className={`tab-txt${tab === 'media' ? ' active' : ''}`} onClick={() => setTab('media')}>Медиа</button>
          <button type="button" className={`tab-txt${tab === 'about' ? ' active' : ''}`} onClick={() => setTab('about')}>О себе</button>
        </div>

        {tab === 'posts' && (
          <UserPosts owner={isMe ? { kind: 'me' } : { kind: 'handle', handle: user.handle }} />
        )}
        {tab === 'media' && <Empty title="Медиа нет" text="Загрузка файлов ещё не реализована." />}
        {tab === 'about' && (
          <div style={{ padding: '20px 0' }}>
            <p><b>Город:</b> {user.city || UNKNOWN}</p>
            <p><b>Работа:</b> {user.work || UNKNOWN}</p>
            <p><b>О себе:</b> {user.bio || UNKNOWN}</p>
          </div>
        )}
      </div>
    </>
  );
}
