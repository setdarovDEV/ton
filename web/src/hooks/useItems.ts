import { useQuery } from '@tanstack/react-query';
import { api } from '@/api/endpoints';
import { keys } from '@/api/queryKeys';

export function useItem(addr: string) {
  return useQuery({
    queryKey: keys.item(addr),
    queryFn: () => api.items.get(addr),
    enabled: !!addr,
    // Item pages show pending txs; a slow poll keeps owner/listing fresh even without WebSocket.
    refetchInterval: (q) => (q.state.data?.transactions.some((t) => t.status === 'pending') ? 4000 : 15000),
  });
}

export function useUserItems(rawAddress: string | null, params: { page: number; limit: number }) {
  return useQuery({
    queryKey: keys.userItems(rawAddress ?? '', params),
    queryFn: () => api.users.items(rawAddress as string, params),
    enabled: !!rawAddress,
    placeholderData: (prev) => prev,
    refetchInterval: 10000,
  });
}
