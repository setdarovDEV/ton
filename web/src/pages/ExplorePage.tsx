import { useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import type { ListingSort } from '@/api/endpoints';
import { useListings } from '@/hooks/useListings';
import { ItemCard } from '@/features/items/ItemCard';
import { BuyButton } from '@/features/listings/BuyButton';
import { CardSkeleton, EmptyState, ErrorState, Grid, Pagination, Select } from '@/components/ui';

const LIMIT = 24;

export function ExplorePage() {
  const [params, setParams] = useSearchParams();
  const page = Math.max(1, Number(params.get('page') ?? 1));
  const [sort, setSort] = useState<ListingSort>((params.get('sort') as ListingSort) || 'newest');
  const q = useListings({ status: 'active', sort, page, limit: LIMIT });

  const setPage = (p: number) => setParams({ page: String(p), sort });
  const changeSort = (s: ListingSort) => {
    setSort(s);
    setParams({ page: '1', sort: s });
  };

  return (
    <section>
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Explore</h1>
          <p className="text-sm text-zinc-400">Fixed-price listings currently active on chain.</p>
        </div>
        <Select<ListingSort>
          label="Sort"
          value={sort}
          onChange={changeSort}
          options={[
            { value: 'newest', label: 'Newest' },
            { value: 'price_asc', label: 'Price: low → high' },
            { value: 'price_desc', label: 'Price: high → low' },
          ]}
        />
      </div>

      {q.isPending ? (
        <Grid>{Array.from({ length: 8 }).map((_, i) => <CardSkeleton key={i} />)}</Grid>
      ) : q.isError ? (
        <ErrorState error={q.error} onRetry={() => q.refetch()} />
      ) : q.data.items.length === 0 ? (
        <EmptyState title="No active listings yet" hint="Connect a testnet wallet, open My NFTs and list something for sale." />
      ) : (
        <>
          <Grid>
            {q.data.items.map((l) =>
              l.item ? (
                <ItemCard key={l.id} item={l.item} listing={l} action={<BuyButton listing={l} className="w-full" />} />
              ) : null,
            )}
          </Grid>
          <Pagination page={q.data.page} limit={q.data.limit} total={q.data.total} onChange={setPage} />
        </>
      )}
    </section>
  );
}
