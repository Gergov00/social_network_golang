import { useState } from 'react';
import { useAuth } from '../../auth/AuthContext';
import { Spinner } from '../../ui/Spinner';
import { EditProfileForm } from './EditProfileForm';
import { ProfileView } from './ProfileView';

export function MePage() {
  const { user, refreshUser } = useAuth();
  const [editing, setEditing] = useState(false);

  if (!user) return <Spinner />;

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
