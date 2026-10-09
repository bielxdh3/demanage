import { Outlet, useLocation } from 'react-router';

import { AppBottomNav } from '@/components/layout/app-bottom-nav';
import { AppSidebar } from '@/components/layout/app-sidebar';
import { AppTopbar } from '@/components/layout/app-topbar';
import { useCards } from '@/hooks/use-cards';
import { useEntries } from '@/hooks/use-entries';
import { useExpenses } from '@/hooks/use-expenses';

export function AppLayout() {
  const { pathname } = useLocation();

  // Only load data used by this route. PiggyBank reads and maintenance run
  // inside Dashboard/PiggyPage, never again in the shared layout.
  useCards(pathname === '/' || pathname === '/despesas');
  useEntries(pathname === '/' || pathname === '/patrimonio');
  useExpenses(pathname === '/' || pathname === '/perfil' || pathname === '/patrimonio');

  return (
    <div className='flex min-h-screen bg-background text-foreground'>
      <AppSidebar />
      <div className='flex min-w-0 flex-1 flex-col'>
        <AppTopbar />
        <main className='relative flex-1 px-4 py-5 pb-24 sm:px-6 md:pb-6 lg:px-8 lg:py-6'>
          <div className='pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_top,rgba(255,184,0,0.04),transparent_35%)]' />
          <div className='relative w-full'>
            <Outlet />
          </div>
        </main>
      </div>
      <AppBottomNav />
    </div>
  );
}
