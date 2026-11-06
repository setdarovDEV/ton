import { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useItem } from '@/hooks/useItems';
import { useAuth } from '@/features/auth/AuthProvider';
import { BuyButton } from '@/features/listings/BuyButton';
import { CancelButton } from '@/features/listings/CancelButton';
import { SellModal } from '@/features/listings/SellModal';
import { NftImage } from '@/components/NftImage';
import { AddressChip } from '@/components/AddressChip';
import { ListingStatusBadge, TxStatusBadge } from '@/components/StatusBadge';
import { Button, ErrorState, Skeleton } from '@/components/ui';
import { explorerTxUrl } from '@/lib/config';
import { formatDate, formatTon, sameRaw, shortHash } from '@/lib/format';
import type { Tx } from '@/lib/types';

export function ItemPage() {
  const { addr = '' } = useParams();
  const q = useItem(addr);
  const { rawAddress, isVerified } = useAuth();
  const [sellOpen, setSellOpen] = useState(false);

  if (q.isPending) {
    return (
      <div className="grid gap-8 md:grid-cols-2">
        <Skeleton className="aspect-square w-full rounded-3xl" />
        <div className="space-y-4">
          <Skeleton className="h-8 w-2/3" />
          <Skeleton className="h-4 w-1/2" />
          <Skeleton className="h-24 w-full" />
        </div>
      </div>
    );
  }
  if (q.isError) return <ErrorState error={q.error} onRetry={() => q.refetch()} />;

  const { item, transactions } = q.data;
  const listing = item.listing;
  const name = item.name || `#${item.index}`;
  const isOwner = sameRaw(rawAddress, item.owner?.raw);
  const canSell = isOwner && isVerified && (!listing || listing.status === 'sold' || listing.status === 'cancelled');
  const attributes = item.metadata.attributes ?? [];

  return (
    <section className="grid gap-8 md:grid-cols-2">
      <NftImage src={item.image_url} alt={name} className="aspect-square w-full rounded-3xl border border-zinc-800" />

      <div className="space-y-6">
        <div>
          {item.collection && (
            <Link to={`/collections/${item.collection.address.raw}`} className="text-sm text-sky-300 hover:underline">
              {item.collection.name}
            </Link>
          )}
          <h1 className="mt-1 text-3xl font-bold">{name}</h1>
          {item.metadata.description && <p className="mt-2 text-sm text-zinc-400">{String(item.metadata.description)}</p>}
        </div>

        <dl className="grid grid-cols-1 gap-2 text-sm sm:grid-cols-2">
          <Row label="Owner">
            <AddressChip addr={item.owner} />
            {isOwner && <span className="ml-2 text-xs text-emerald-300">you</span>}
          </Row>
          <Row label="Item">
            <AddressChip addr={item.address} />
          </Row>
          <Row label="Index">#{item.index}</Row>
          <Row label="Updated">{formatDate(item.updated_at)}</Row>
        </dl>

        <div className="rounded-2xl border border-zinc-800 bg-zinc-900/60 p-5">
          {listing && listing.status !== 'sold' && listing.status !== 'cancelled' ? (
            <>
              <div className="flex items-center justify-between">
                <span className="text-sm text-zinc-400">Price</span>
                <ListingStatusBadge status={listing.status} />
              </div>
              <p className="mt-1 text-3xl font-bold">{formatTon(listing.price_nano)} TON</p>
              <div className="mt-2 flex flex-wrap gap-4 text-xs text-zinc-500">
                <AddressChip addr={listing.seller} label="Seller" />
                <AddressChip addr={listing.sale_contract} label="Sale contract" />
              </div>
              {listing.status === 'pending' && (
                <p className="mt-3 text-xs text-amber-300">Listing is pending: waiting for the NFT to arrive at the sale contract.</p>
              )}
              <div className="mt-4 flex flex-wrap gap-2">
                <BuyButton listing={listing} className="flex-1" />
                <CancelButton listing={listing} className="flex-1" />
              </div>
            </>
          ) : (
            <>
              <p className="text-sm text-zinc-400">Not listed for sale</p>
              {canSell ? (
                <Button className="mt-3 w-full" onClick={() => setSellOpen(true)}>
                  Sell
                </Button>
              ) : isOwner && !isVerified ? (
                <p className="mt-3 text-xs text-amber-300">Reconnect your wallet to sign ton_proof before selling.</p>
              ) : null}
            </>
          )}
        </div>

        {attributes.length > 0 && (
          <div>
            <h2 className="mb-2 text-sm font-semibold text-zinc-300">Attributes</h2>
            <ul className="grid grid-cols-2 gap-2 sm:grid-cols-3">
              {attributes.map((a, i) => (
                <li key={`${a.trait_type}-${i}`} className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-3">
                  <p className="truncate text-[11px] uppercase tracking-wide text-zinc-500">{a.trait_type}</p>
                  <p className="truncate text-sm font-medium">{String(a.value)}</p>
                </li>
              ))}
            </ul>
          </div>
        )}

        <div>
          <h2 className="mb-2 text-sm font-semibold text-zinc-300">Transaction history</h2>
          {transactions.length === 0 ? (
            <p className="text-sm text-zinc-500">No marketplace transactions indexed for this item yet.</p>
          ) : (
            <ul className="divide-y divide-zinc-800 overflow-hidden rounded-2xl border border-zinc-800">
              {transactions.map((tx) => (
                <TxRow key={tx.hash} tx={tx} />
              ))}
            </ul>
          )}
        </div>
      </div>

      <SellModal item={item} open={sellOpen} onClose={() => setSellOpen(false)} />
    </section>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-xl border border-zinc-800/80 bg-zinc-900/40 px-3 py-2">
      <dt className="text-zinc-500">{label}</dt>
      <dd className="min-w-0 truncate text-right">{children}</dd>
    </div>
  );
}

function TxRow({ tx }: { tx: Tx }) {
  return (
    <li className="flex flex-wrap items-center gap-3 bg-zinc-900/40 px-4 py-3 text-sm">
      <span className="w-16 font-medium capitalize">{tx.type}</span>
      <TxStatusBadge status={tx.status} />
      <a href={explorerTxUrl(tx.hash)} target="_blank" rel="noreferrer" className="font-mono text-xs text-sky-300 hover:underline">
        {shortHash(tx.hash)}
      </a>
      {tx.amount_nano && <span className="text-zinc-300">{formatTon(tx.amount_nano)} TON</span>}
      <span className="ml-auto text-xs text-zinc-500">{formatDate(tx.seen_at)}</span>
    </li>
  );
}
