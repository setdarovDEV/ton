import { useState } from 'react';
import { useTonConnectModal } from '@tonconnect/ui-react';
import { api } from '@/api/endpoints';
import type { Listing } from '@/lib/types';
import { formatTon, sameRaw } from '@/lib/format';
import { Button } from '@/components/ui';
import { Modal } from '@/components/Modal';
import { useAuth } from '@/features/auth/AuthProvider';
import { useSendIntent } from '@/features/tx/useSendIntent';
import { TxTracker } from '@/features/tx/TxTracker';

/** Buy flow: backend builds the message to the sale contract (price + gas); excess gas is refunded by the contract. */
export function BuyButton({ listing, className = '' }: { listing: Listing; className?: string }) {
  const { isVerified, rawAddress } = useAuth();
  const { open: openWalletModal } = useTonConnectModal();
  const flow = useSendIntent();
  const [open, setOpen] = useState(false);

  const isSeller = sameRaw(rawAddress, listing.seller.raw);
  if (listing.status !== 'active' || isSeller) return null;

  const start = async () => {
    if (!isVerified) {
      openWalletModal();
      return;
    }
    setOpen(true);
    await flow.run(
      () => api.listings.buyIntent(listing.id),
      (_intent, boc) => ({ boc, type: 'buy', nft_address: listing.nft_address.raw, listing_id: listing.id }),
    );
  };

  const close = () => {
    setOpen(false);
    flow.reset();
  };

  return (
    <>
      <Button className={className} onClick={start}>
        Buy for {formatTon(listing.price_nano)} TON
      </Button>
      <Modal open={open} onClose={close} title="Buying NFT">
        <p className="mb-4 text-sm text-zinc-400">
          You pay <span className="font-semibold text-zinc-100">{formatTon(listing.price_nano)} TON</span> plus ~0.1 TON for gas. Unused gas is
          returned to your wallet by the sale contract.
        </p>
        <TxTracker step={flow.step} tx={flow.tx} error={flow.error} />
        <div className="mt-4 flex justify-end">
          <Button variant="secondary" onClick={close}>
            Close
          </Button>
        </div>
      </Modal>
    </>
  );
}
