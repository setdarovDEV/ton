import { useState } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import type { ItemSort } from '@/api/endpoints';
import { useCollection, useCollectionItems } from '@/hooks/useCollections';
import { ItemCard } from '@/features/items/ItemCard';
import { BuyButton } from '@/features/listings/BuyButton';
import { NftImage } from '@/components/NftImage';
import { AddressChip } from '@/components/AddressChip';
import { CardSkeleton, EmptyState, ErrorState, Grid, Pagination, Select, Skeleton } from '@/components/ui';

const LIMIT = 24;

export function CollectionPage() {
  const { addr = '' } = useParams();
  const [params, setParams] = useSearchParams();
  const page = Math.max(1, Number(params.get('page') ?? 1));
  const [sort, setSort] = useState<ItemSort>((params.get('sort') as ItemSort) || 'index');

  const col = useCollection(addr);
  const items = useCollectionItems(addr, { page, limit: LIMIT, sort });

  return (
    <section>
      <header className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center">
        {col.isPending ? (
          <Skeleton className="h-20 w-20 rounded-2xl" />
        ) : (
          <NftImage src={col.data?.image_url} alt={col.data?.name ?? ''} className="h-20 w-20 shrink-0 rounded-2xl" />
        )}
        <div className="min-w-0 flex-1">
          {col.isPending ? (
            <Skeleton className="h-7 w-48" />
          ) : col.isError ? (
            <ErrorState error={col.error} onRetry={() => col.refetch()} />
          ) : (
            <>
              <h1 className="truncate text-2xl font-bold">{col.data.name || 'Untitled collection'}</h1>
              <p className="line-clamp-2 text-sm text-zinc-400">{col.data.description}</p>
              <div className="mt-1 flex flex-wrap gap-4 text-xs text-zinc-500">
                <AddressChip addr={col.data.address} label="Collection" />
                <AddressChip addr={col.data.owner} label="Owner" />
                <span>{col.data.items_count ?? col.data.next_item_index} items</span>
              </div>
            </>
          )}
        </div>
        <Select<ItemSort>
          label="Sort"
          value={sort}
          onChange={(s) => {
            setSort(s);
            setParams({ page: '1', sort: s });
          }}
          options={[
            { value: 'index', label: 'Index' },
            { value: 'price_asc', label: 'Price: low → high' },
            { value: 'price_desc', label: 'Price: high → low' },
          ]}
        />
      </header>

      {items.isPending ? (
        <Grid>{Array.from({ length: 8 }).map((_, i) => <CardSkeleton key={i} />)}</Grid>
      ) : items.isError ? (
        <ErrorState error={items.error} onRetry={() => items.refetch()} />
      ) : items.data.items.length === 0 ? (
        <EmptyState title="No items indexed yet" hint="The indexer syncs items every few seconds after a collection is added." />
      ) : (
        <>
          <Grid>
            {items.data.items.map((it) => (
              <ItemCard key={it.address.raw} item={it} listing={it.listing} action={it.listing ? <BuyButton listing={it.listing} className="w-full" /> : undefined} />
            ))}
          </Grid>
          <Pagination page={items.data.page} limit={items.data.limit} total={items.data.total} onChange={(p) => setParams({ page: String(p), sort })} />
        </>
      )}
    </section>
  );
}
