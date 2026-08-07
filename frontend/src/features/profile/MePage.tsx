import { useState } from 'react';
import { useAuth } from '../../auth/AuthContext';
import { ErrorBox } from '../../ui/ErrorBox';
import { Spinner } from '../../ui/Spinner';
import { EditProfileForm } from './EditProfileForm';
import { ProfileView } from './ProfileView';

export function MePage() {
  const { user, meError, refreshUser } = useAuth();
  const [editing, setEditing] = useState(false);

  // user===null при status==='authed' — законное состояние (см.
  // AuthContext.resolveMe): профиль ещё не создан либо /me моргнул. meError
  // всегда заполнен в этом случае — показываем его вместо вечного спиннера.
  // Голый спиннер остаётся только на случай самого первого рендера, пока
  // meError ещё не проставлен.
  if (!user) {
    if (meError) return <ErrorBox message={meError} onRetry={() => void refreshUser()} />;
    return <Spinner />;
  }

  return (
    <div className="screen">
      <div className="profile-wrap">
        <ProfileView user={user} isMe onEdit={() => setEditing(true)} />
        {editing && (
          <EditProfileForm
            user={user}
            onCancel={() => setEditing(false)}
            onDone={async () => {
              await refreshUser();
              setEditing(false);
            }}
          />
        )}
      </div>
    </div>
  );
}
