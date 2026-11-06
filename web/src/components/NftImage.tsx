import { useState } from 'react';

export function NftImage({ src, alt, className = '' }: { src?: string | null; alt: string; className?: string }) {
  const [failed, setFailed] = useState(false);
  const url = src && !failed ? resolveImage(src) : null;
  return (
    <div className={`relative overflow-hidden bg-gradient-to-br from-zinc-800 to-zinc-900 ${className}`}>
      {url ? (
        <img src={url} alt={alt} loading="lazy" onError={() => setFailed(true)} className="h-full w-full object-cover" />
      ) : (
        <div className="flex h-full w-full items-center justify-center text-4xl text-zinc-600" aria-hidden="true">
          ◇
        </div>
      )}
    </div>
  );
}

/** Rewrites ipfs:// URIs to a public gateway. */
function resolveImage(src: string): string {
  return src.startsWith('ipfs://') ? `https://ipfs.io/ipfs/${src.slice('ipfs://'.length)}` : src;
}
