/** Runtime configuration read from Vite env. All values have same-origin defaults. */
export const config = {
  /** Base URL of the Go API; empty string means same-origin. */
  apiUrl: (import.meta.env.VITE_API_URL as string | undefined) ?? '',
  network: ((import.meta.env.VITE_TON_NETWORK as string | undefined) ?? 'testnet') as 'testnet' | 'mainnet',
} as const;

export const explorerBase =
  config.network === 'mainnet' ? 'https://tonviewer.com' : 'https://testnet.tonviewer.com';

export const explorerAddressUrl = (address: string) => `${explorerBase}/${address}`;
export const explorerTxUrl = (hash: string) => `${explorerBase}/transaction/${hash}`;

/** TON Connect chain id strings: -239 mainnet, -3 testnet. */
export const expectedChainId = config.network === 'mainnet' ? '-239' : '-3';

export const manifestUrl = `${window.location.origin}/tonconnect-manifest.json`;

export const wsUrl = (() => {
  const base = config.apiUrl ? new URL(config.apiUrl, window.location.origin) : new URL(window.location.origin);
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:';
  base.pathname = '/ws';
  base.search = '';
  return base.toString();
})();
