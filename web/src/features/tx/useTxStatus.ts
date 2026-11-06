import { useQuery } from '@tanstack/react-query';
import { api } from '@/api/endpoints';
import { keys } from '@/api/queryKeys';

const POLL_MS = 3000;

/** Polls a registered transaction until it leaves the `pending` state. */
export function useTxStatus(hash: string | null) {
  return useQuery({
    queryKey: keys.tx(hash ?? ''),
    queryFn: () => api.tx.get(hash as string),
    enabled: !!hash,
    refetchInterval: (query) => (query.state.data && query.state.data.status !== 'pending' ? false : POLL_MS),
    refetchIntervalInBackground: true,
  });
}
