import { Link } from 'react-router-dom';
import type { Item, ItemSummary, Listing } from '@/lib/types';
import { formatTon } from '@/lib/format';
import { NftImage } from '@/components/NftImage';
import { ListingStatusBadge } from '@/components/StatusBadge';
import type { ReactNode } from 'react';

interface Props {
  item: ItemSummary | Item;
  listing?: Listing | null;
  /** Action area rendered under the price (Buy / Sell / Cancel). */
  action?: ReactNode;
}

export function ItemCard({ item, listing, action }: Props) {
  const name = item.name || `#${item.index}`;
  return (
    <article className="group flex flex-col overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/60 transition hover:-translate-y-0.5 hover:border-zinc-700 hover:shadow-xl hover:shadow-black/40">
      <Link to={`/items/${item.address.raw}`} className="block focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400">
        <NftImage src={item.image_url} alt={name} className="aspect-square w-full transition group-hover:scale-[1.02]" />
      </Link>
      <div className="flex flex-1 flex-col gap-2 p-4">
        <div className="min-w-0">
          {item.collection && (
            <Link to={`/collections/${item.collection.address.raw}`} className="block truncate text-xs text-zinc-500 hover:text-sky-300">
              {item.collection.name}
            </Link>
          )}
          <Link to={`/items/${item.address.raw}`} className="block truncate font-semibold text-zinc-100 hover:text-sky-300">
            {name}
          </Link>
        </div>
        <div className="mt-auto flex items-center justify-between">
          {listing ? (
            <div className="flex items-center gap-2">
              <span className="text-sm font-semibold text-zinc-100">{formatTon(listing.price_nano)} TON</span>
              {listing.status !== 'active' && <ListingStatusBadge status={listing.status} />}
            </div>
          ) : (
            <span className="text-sm text-zinc-500">Not listed</span>
          )}
        </div>
        {action && <div className="pt-1">{action}</div>}
      </div>
    </article>
  );
}
