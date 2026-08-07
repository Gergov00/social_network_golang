// frontend/src/app/AppShell.tsx (временная заглушка Task 4; Task 5 заменит на рейл + шапку)
import { Outlet } from 'react-router-dom';

export function AppShell() {
  return <div className="app"><div className="main"><Outlet /></div></div>;
}
