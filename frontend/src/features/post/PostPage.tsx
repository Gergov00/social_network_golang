import { useQuery } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import { ApiError } from '../../api/client';
import { postsApi } from '../../api/posts';
import { Empty } from '../../ui/Empty';
import { ErrorBox } from '../../ui/ErrorBox';
import { Spinner } from '../../ui/Spinner';
import { PostCard } from '../feed/PostCard';
import { CommentComposer } from './CommentComposer';
import { CommentList } from './CommentList';

export function PostPage() {
  const { id = '' } = useParams();
  const query = useQuery({
    queryKey: ['post', id],
    queryFn: () => postsApi.getPost(id),
    enabled: id !== '',
  });

  const notFound = query.error instanceof ApiError && query.error.status === 404;

  return (
    <div className="screen">
      <div className="center-col">
        {query.isPending && <Spinner />}
        {notFound && <Empty title="Запись не найдена" />}
        {query.isError && !notFound && (
          <ErrorBox message="Не удалось загрузить запись" onRetry={() => void query.refetch()} />
        )}
        {query.data && (
          <>
            <PostCard post={query.data} detail />
            <CommentComposer postId={id} />
            <CommentList postId={id} />
          </>
        )}
      </div>
    </div>
  );
}
