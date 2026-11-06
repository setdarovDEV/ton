import { useQuery } from '@tanstack/react-query';
import { api, type ListingSort } from '@/api/endpoints';
import { keys } from '@/api/queryKeys';
import type { ListingStatus } from '@/lib/types';

export function useListings(params: { status: ListingStatus; sort: ListingSort; page: number; limit: number }) {
  return useQuery({
    queryKey: keys.listings(params),
    queryFn: () => api.listings.list(params),
    placeholderData: (prev) => prev,
    refetchInterval: 10000,
  });
}
