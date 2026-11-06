import { Badge } from './ui';
import type { ListingStatus, TxStatus } from '@/lib/types';

const txTone: Record<TxStatus, 'amber' | 'emerald' | 'rose'> = { pending: 'amber', confirmed: 'emerald', failed: 'rose' };
const listingTone: Record<ListingStatus, 'amber' | 'sky' | 'emerald' | 'zinc'> = {
  pending: 'amber',
  active: 'sky',
  sold: 'emerald',
  cancelled: 'zinc',
};

export function TxStatusBadge({ status }: { status: TxStatus }) {
  return <Badge tone={txTone[status]}>{status}</Badge>;
}

export function ListingStatusBadge({ status }: { status: ListingStatus }) {
  return <Badge tone={listingTone[status]}>{status}</Badge>;
}
