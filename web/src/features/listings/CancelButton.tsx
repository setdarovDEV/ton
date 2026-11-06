import { useState } from 'react';
import { api } from '@/api/endpoints';
import type { Listing } from '@/lib/types';
import { Button } from '@/components/ui';
import { Modal } from '@/components/Modal';
import { useAuth } from '@/features/auth/AuthProvider';
import { useSendIntent } from '@/features/tx/useSendIntent';
import { TxTracker } from '@/features/tx/TxTracker';

/** Cancel flow: seller sends op=3 to the sale contract, which returns the NFT. */
export function CancelButton({ listing, className = '' }: { listing: Listing; className?: string }) {
  const { isVerified, rawAddress } = useAuth();
  const flow = useSendIntent();
  const [open, setOpen] = useState(false);

  const isSeller = rawAddress && rawAddress.toLowerCase() === listing.seller.raw.toLowerCase();
  if (listing.status !== 'active' || !isSeller || !isVerified) return null;

  const start = async () => {
    setOpen(true);
    await flow.run(
      () => api.listings.cancelIntent(listing.id),
      (_intent, boc) => ({ boc, type: 'cancel', nft_address: listing.nft_address.raw, listing_id: listing.id }),
    );
  };

  const close = () => {
    setOpen(false);
    flow.reset();
  };

  return (
    <>
      <Button variant="danger" className={className} onClick={start}>
        Cancel listing
      </Button>
      <Modal open={open} onClose={close} title="Cancelling listing">
        <p className="mb-4 text-sm text-zinc-400">The sale contract will transfer the NFT back to your wallet (~0.1 TON gas).</p>
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
