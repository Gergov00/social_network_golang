import { Navigate, Outlet } from 'react-router-dom';
import { Spinner } from '../ui/Spinner';
import { useAuth } from './AuthContext';

// Пока статус не определён, не рендерим НИ защищённое, ни логин: иначе у
// вошедшего пользователя на каждой перезагрузке мигает форма входа.
export function ProtectedRoute() {
  const { status } = useAuth();

  if (status === 'loading') return <Spinner />;
  if (status === 'anon') return <Navigate to="/login" replace />;
  return <Outlet />;
}
