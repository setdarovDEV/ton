import { useState } from 'react';
import { useTonConnectModal } from '@tonconnect/ui-react';
import { useAuth } from '@/features/auth/AuthProvider';
import { useUserItems } from '@/hooks/useItems';
import { ItemCard } from '@/features/items/ItemCard';
import { SellModal } from '@/features/listings/SellModal';
import { CancelButton } from '@/features/listings/CancelButton';
import { Button, CardSkeleton, EmptyState, ErrorState, Grid, Pagination } from '@/components/ui';
import type { Item } from '@/lib/types';

const LIMIT = 24;

export function MyNftsPage() {
  const { rawAddress, isVerified, wallet } = useAuth();
  const { open } = useTonConnectModal();
  const [page, setPage] = useState(1);
  const [selling, setSelling] = useState<Item | null>(null);
  const q = useUserItems(rawAddress, { page, limit: LIMIT });

  if (!wallet) {
    return (
      <EmptyState
        title="Connect your wallet"
        hint="Use Tonkeeper or MyTonWallet in testnet mode. Signing ton_proof lets the backend issue you a session token."
        action={<Button onClick={() => open()}>Connect wallet</Button>}
      />
    );
  }

  return (
    <section>
      <div className="mb-6">
        <h1 className="text-2xl font-bold">My NFTs</h1>
        <p className="text-sm text-zinc-400">
          Items owned by your wallet, plus items you have listed (held by your sale contracts).
          {!isVerified && <span className="ml-2 text-amber-300">Reconnect to verify ton_proof before selling.</span>}
        </p>
      </div>
      {q.isPending ? (
        <Grid>{Array.from({ length: 4 }).map((_, i) => <CardSkeleton key={i} />)}</Grid>
      ) : q.isError ? (
        <ErrorState error={q.error} onRetry={() => q.refetch()} />
      ) : q.data.items.length === 0 ? (
        <EmptyState title="No NFTs found" hint="Mint items from a tracked collection to your wallet, or buy one on Explore." />
      ) : (
        <>
          <Grid>
            {q.data.items.map((it) => (
              <ItemCard
                key={it.address.raw}
                item={it}
                listing={it.listing}
                action={
                  it.listing && it.listing.status === 'active' ? (
                    <CancelButton listing={it.listing} className="w-full" />
                  ) : it.listing && it.listing.status === 'pending' ? (
                    <Button variant="secondary" className="w-full" disabled>
                      Listing pending…
                    </Button>
                  ) : (
                    <Button className="w-full" disabled={!isVerified} onClick={() => setSelling(it)}>
                      Sell
                    </Button>
                  )
                }
              />
            ))}
          </Grid>
          <Pagination page={q.data.page} limit={q.data.limit} total={q.data.total} onChange={setPage} />
        </>
      )}
      {selling && <SellModal item={selling} open onClose={() => setSelling(null)} />}
    </section>
  );
}
