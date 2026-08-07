import { Navigate, Route, Routes } from 'react-router-dom';
import { AppShell } from './AppShell';
import { ProtectedRoute } from '../auth/ProtectedRoute';
import { AuthPage } from '../features/auth/AuthPage';
import { FeedPage } from '../features/feed/FeedPage';
import { PostPage } from '../features/post/PostPage';
import { MePage } from '../features/profile/MePage';
import { UserPage } from '../features/profile/UserPage';

export function AppRouter() {
  return (
    <Routes>
      <Route path="/login" element={<AuthPage mode="login" />} />
      <Route path="/register" element={<AuthPage mode="register" />} />

      {/* Два вложенных layout-маршрута: ProtectedRoute решает, пускать ли,
          AppShell рисует рейл и шапку один раз на все защищённые экраны. */}
      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route path="/" element={<FeedPage />} />
          <Route path="/post/:id" element={<PostPage />} />
          <Route path="/me" element={<MePage />} />
          <Route path="/u/:handle" element={<UserPage />} />
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
