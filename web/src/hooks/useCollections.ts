import { useQuery } from '@tanstack/react-query';
import { api, type ItemSort } from '@/api/endpoints';
import { keys } from '@/api/queryKeys';

export function useCollections(params: { page: number; limit: number }) {
  return useQuery({
    queryKey: keys.collectionsList(params),
    queryFn: () => api.collections.list(params),
    placeholderData: (prev) => prev,
  });
}

export function useCollection(addr: string) {
  return useQuery({ queryKey: keys.collection(addr), queryFn: () => api.collections.get(addr), enabled: !!addr });
}

export function useCollectionItems(addr: string, params: { page: number; limit: number; sort: ItemSort }) {
  return useQuery({
    queryKey: keys.collectionItems(addr, params),
    queryFn: () => api.collections.items(addr, params),
    enabled: !!addr,
    placeholderData: (prev) => prev,
  });
}
