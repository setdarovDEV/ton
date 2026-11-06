import { useState, type FormEvent } from 'react';
import { api } from '@/api/endpoints';
import type { Item } from '@/lib/types';
import { formatTon, parseTon } from '@/lib/format';
import { Button } from '@/components/ui';
import { Modal } from '@/components/Modal';
import { useSendIntent } from '@/features/tx/useSendIntent';
import { TxTracker } from '@/features/tx/TxTracker';

/**
 * Listing flow (Getgems fixprice v3 pattern):
 * backend returns two messages — deploy sale contract through the marketplace + transfer NFT to the sale —
 * which the wallet signs in a single TON Connect transaction.
 */
export function SellModal({ item, open, onClose }: { item: Item; open: boolean; onClose: () => void }) {
  const [price, setPrice] = useState('');
  const [formError, setFormError] = useState<string | null>(null);
  const flow = useSendIntent();

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setFormError(null);
    let nano: bigint;
    try {
      nano = parseTon(price);
      if (nano <= 0n) throw new Error('Price must be greater than zero');
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Invalid price');
      return;
    }
    await flow.run(
      () => api.listings.intent({ nft_address: item.address.raw, price_nano: nano.toString() }),
      (intent, boc) => ({ boc, type: 'list', nft_address: item.address.raw, listing_id: intent.listing_id }),
    );
  };

  const close = () => {
    flow.reset();
    setPrice('');
    setFormError(null);
    onClose();
  };

  const busy = flow.step !== 'idle' && flow.step !== 'error';

  return (
    <Modal open={open} onClose={close} title={`Sell “${item.name || `#${item.index}`}”`}>
      {flow.step === 'idle' || flow.step === 'error' ? (
        <form onSubmit={submit} className="space-y-4">
          <label className="block text-sm">
            <span className="mb-1 block text-zinc-400">Price (TON)</span>
            <input
              inputMode="decimal"
              autoFocus
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              placeholder="1.5"
              className="w-full rounded-xl border border-zinc-700 bg-zinc-900 px-3 py-2 text-lg text-zinc-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400"
            />
          </label>
          {formError && <p className="text-sm text-rose-300">{formError}</p>}
          {flow.step === 'error' && flow.error && <p className="text-sm text-rose-300">{flow.error}</p>}
          <p className="text-xs text-zinc-500">
            Your wallet will sign one transaction with two messages: deploy the fixed-price sale contract and transfer the NFT to it
            (~0.3 TON reserved for gas and storage; excess is returned).
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit">List for sale</Button>
          </div>
        </form>
      ) : (
        <div className="space-y-4">
          <p className="text-sm text-zinc-400">
            Listing for <span className="font-semibold text-zinc-100">{formatTon(parseTonSafe(price))} TON</span>
          </p>
          <TxTracker step={flow.step} tx={flow.tx} error={flow.error} />
          <div className="flex justify-end">
            <Button variant="secondary" onClick={close} disabled={busy && flow.step !== 'tracking'}>
              Close
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

function parseTonSafe(v: string): bigint {
  try {
    return parseTon(v);
  } catch {
    return 0n;
  }
}
