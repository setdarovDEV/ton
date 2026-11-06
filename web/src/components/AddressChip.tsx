import { useState } from 'react';
import { explorerAddressUrl } from '@/lib/config';
import { shortAddr } from '@/lib/format';
import type { Addr } from '@/lib/types';

export function AddressChip({ addr, label }: { addr: Addr | null | undefined; label?: string }) {
  const [copied, setCopied] = useState(false);
  if (!addr) return <span className="text-sm text-zinc-500">{label ? `${label}: ` : ''}—</span>;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(addr.address);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard unavailable */
    }
  };

  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      {label && <span className="text-zinc-500">{label}:</span>}
      <a
        href={explorerAddressUrl(addr.address)}
        target="_blank"
        rel="noreferrer"
        className="font-mono text-sky-300 hover:underline"
        title={addr.address}
      >
        {shortAddr(addr.address)}
      </a>
      <button
        type="button"
        onClick={copy}
        aria-label="Copy address"
        className="rounded px-1 text-xs text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200"
      >
        {copied ? '✓' : '⧉'}
      </button>
    </span>
  );
}
