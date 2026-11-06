import { NavLink, Outlet } from 'react-router-dom';
import { TonConnectButton } from '@tonconnect/ui-react';
import { useAuth } from '@/features/auth/AuthProvider';
import { useLiveUpdates } from '@/features/tx/useLiveUpdates';
import { config } from '@/lib/config';

const nav = [
  { to: '/', label: 'Explore' },
  { to: '/collections', label: 'Collections' },
  { to: '/me', label: 'My NFTs' },
];

export function Layout() {
  const { isVerified, wallet } = useAuth();
  useLiveUpdates();

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-30 border-b border-zinc-800/80 bg-[#0b0e14]/80 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center gap-4 px-4 py-3 sm:px-6">
          <NavLink to="/" className="flex items-center gap-2 text-lg font-bold tracking-tight">
            <span className="inline-block h-6 w-6 rounded-md bg-gradient-to-br from-sky-400 to-indigo-500" aria-hidden="true" />
            TONNET
          </NavLink>
          <span className="rounded-full border border-amber-500/40 bg-amber-500/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-amber-300">
            {config.network}
          </span>
          <nav className="ml-2 hidden items-center gap-1 sm:flex" aria-label="Main">
            {nav.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.to === '/'}
                className={({ isActive }) =>
                  `rounded-lg px-3 py-1.5 text-sm font-medium transition ${isActive ? 'bg-zinc-800 text-white' : 'text-zinc-400 hover:text-zinc-100'}`
                }
              >
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-2">
            {wallet && (
              <span
                title={isVerified ? 'ton_proof verified by backend' : 'Reconnect to sign ton_proof'}
                className={`hidden text-xs sm:inline ${isVerified ? 'text-emerald-300' : 'text-amber-300'}`}
              >
                {isVerified ? '✓ verified' : '⚠ unverified'}
              </span>
            )}
            <TonConnectButton />
          </div>
        </div>
        <nav className="flex gap-1 border-t border-zinc-800/60 px-2 py-1 sm:hidden" aria-label="Main mobile">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.to === '/'}
              className={({ isActive }) =>
                `flex-1 rounded-lg px-3 py-1.5 text-center text-sm ${isActive ? 'bg-zinc-800 text-white' : 'text-zinc-400'}`
              }
            >
              {n.label}
            </NavLink>
          ))}
        </nav>
      </header>
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6">
        <Outlet />
      </main>
      <footer className="border-t border-zinc-800/80 py-6 text-center text-xs text-zinc-500">
        Demo marketplace on TON testnet · standard Getgems contracts (TEP-62 / TEP-64) · DB is the source of truth for the UI, the chain
        for ownership.
      </footer>
    </div>
  );
}
