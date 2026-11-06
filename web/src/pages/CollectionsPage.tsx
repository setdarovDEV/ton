import { Link, useSearchParams } from "react-router-dom";
import { useCollections } from "@/hooks/useCollections";
import { NftImage } from "@/components/NftImage";
import { AddressChip } from "@/components/AddressChip";
import {
  CardSkeleton,
  EmptyState,
  ErrorState,
  Grid,
  Pagination,
} from "@/components/ui";

const LIMIT = 24;

export function CollectionsPage() {
  const [params, setParams] = useSearchParams();
  const page = Math.max(1, Number(params.get("page") ?? 1));
  const q = useCollections({ page, limit: LIMIT });
  return (
    <section>
      <h1 className="mb-1 text-2xl font-bold">Collections</h1>
      <p className="mb-6 text-sm text-zinc-400">
        TEP-62 collections tracked by the indexer.
      </p>
      {q.isPending ? (
        <Grid>
          {Array.from({ length: 4 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </Grid>
      ) : q.isError ? (
        <ErrorState error={q.error} onRetry={() => q.refetch()} />
      ) : q.data.items.length === 0 ? (
        <EmptyState
          title="No collections indexed"
          hint="Add a collection address to the indexer configuration (COLLECTIONS env)."
        />
      ) : (
        <>
          <Grid>
            {q.data.items.map((c) => (
              <article
                key={c.address.raw}
                className="overflow-hidden rounded-2xl border border-zinc-800 bg-zinc-900/60 transition hover:border-zinc-700"
              >
                <Link to={`/collections/${c.address.raw}`}>
                  <NftImage
                    src={c.image_url}
                    alt={c.name}
                    className="aspect-[4/3] w-full"
                  />
                </Link>
                <div className="space-y-1 p-4">
                  <Link
                    to={`/collections/${c.address.raw}`}
                    className="block truncate font-semibold hover:text-sky-300"
                  >
                    {c.name || "Untitled collection"}
                  </Link>
                  <p className="line-clamp-2 text-xs text-zinc-400">
                    {c.description}
                  </p>
                  <div className="flex items-center justify-between pt-2 text-xs text-zinc-500">
                    <span>{c.items_count ?? c.next_item_index} items</span>
                    <AddressChip addr={c.address} />
                  </div>
                </div>
              </article>
            ))}
          </Grid>
          <Pagination
            page={q.data.page}
            limit={q.data.limit}
            total={q.data.total}
            onChange={(p) => setParams({ page: String(p) })}
          />
        </>
      )}
    </section>
  );
}
