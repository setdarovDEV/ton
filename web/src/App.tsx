import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TonConnectUIProvider, THEME } from '@tonconnect/ui-react';
import { manifestUrl } from '@/lib/config';
import { ToastProvider } from '@/components/Toast';
import { AuthProvider } from '@/features/auth/AuthProvider';
import { Layout } from '@/components/Layout';
import { ExplorePage } from '@/pages/ExplorePage';
import { CollectionsPage } from '@/pages/CollectionsPage';
import { CollectionPage } from '@/pages/CollectionPage';
import { ItemPage } from '@/pages/ItemPage';
import { MyNftsPage } from '@/pages/MyNftsPage';
import { NotFoundPage } from '@/pages/NotFoundPage';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5_000, retry: 1, refetchOnWindowFocus: true },
  },
});

export default function App() {
  return (
    <TonConnectUIProvider manifestUrl={manifestUrl} uiPreferences={{ theme: THEME.DARK }}>
      <QueryClientProvider client={queryClient}>
        <ToastProvider>
          <AuthProvider>
            <BrowserRouter>
              <Routes>
                <Route element={<Layout />}>
                  <Route index element={<ExplorePage />} />
                  <Route path="collections" element={<CollectionsPage />} />
                  <Route path="collections/:addr" element={<CollectionPage />} />
                  <Route path="items/:addr" element={<ItemPage />} />
                  <Route path="me" element={<MyNftsPage />} />
                  <Route path="*" element={<NotFoundPage />} />
                </Route>
              </Routes>
            </BrowserRouter>
          </AuthProvider>
        </ToastProvider>
      </QueryClientProvider>
    </TonConnectUIProvider>
  );
}
