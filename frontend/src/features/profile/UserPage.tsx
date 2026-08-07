import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { ApiError } from '../../api/client';
import { usersApi } from '../../api/users';
import { Empty } from '../../ui/Empty';
import { ErrorBox } from '../../ui/ErrorBox';
import { Spinner } from '../../ui/Spinner';
import { ProfileView } from './ProfileView';

export function UserPage() {
  const { handle = '' } = useParams();
  const query = useQuery({
    queryKey: ['user', handle],
    queryFn: () => usersApi.getUserByHandle(handle),
    enabled: handle !== '',
  });

  const notFound = query.error instanceof ApiError && query.error.status === 404;

  return (
    <div className="screen">
      <div className="profile-wrap">
        {query.isPending && <Spinner />}
        {notFound && <Empty title="Пользователь не найден" text={`Профиля @${handle} не существует.`} />}
        {query.isError && !notFound && (
          <ErrorBox message="Не удалось загрузить профиль" onRetry={() => void query.refetch()} />
        )}
        {query.data && <ProfileView user={query.data} isMe={false} />}
      </div>
    </div>
  );
}
