/**
 * API DTOs shared with the Go backend (internal/http).
 * Addresses always come as both user-friendly (bounceable, testnet-flagged) and raw ("0:hex").
 * Amounts in nanoTON are serialised as decimal strings to avoid JS number precision loss.
 */
export interface Addr {
  address: string;
  raw: string;
}

export type ListingStatus = 'pending' | 'active' | 'sold' | 'cancelled';
export type TxStatus = 'pending' | 'confirmed' | 'failed';
export type TxType = 'list' | 'buy' | 'cancel' | 'transfer' | 'mint';

export interface Page<T> {
  items: T[];
  page: number;
  limit: number;
  total: number;
}

export interface Collection {
  address: Addr;
  owner: Addr | null;
  name: string;
  description: string;
  image_url: string;
  metadata: Record<string, unknown>;
  next_item_index: number;
  items_count: number;
  indexed_at: string;
}

export interface CollectionRef {
  address: Addr;
  name: string;
}

export interface Listing {
  id: number;
  nft_address: Addr;
  sale_contract: Addr;
  seller: Addr;
  price_nano: string;
  status: ListingStatus;
  created_at: string;
  updated_at: string;
  /** Present on /api/listings so Explore can render cards without extra calls. */
  item?: ItemSummary;
}

export interface ItemSummary {
  address: Addr;
  index: number;
  name: string;
  image_url: string;
  collection: CollectionRef | null;
}

export interface Item extends ItemSummary {
  owner: Addr | null;
  metadata: ItemMetadata;
  listing: Listing | null;
  updated_at: string;
}

export interface ItemMetadata {
  name?: string;
  description?: string;
  image?: string;
  attributes?: Array<{ trait_type: string; value: string | number }>;
  [k: string]: unknown;
}

export interface ItemDetail {
  item: Item;
  transactions: Tx[];
}

export interface Tx {
  hash: string;
  type: TxType;
  nft_address: Addr | null;
  from: Addr | null;
  to: Addr | null;
  amount_nano: string | null;
  lt: number | null;
  status: TxStatus;
  seen_at: string;
}

/** One TON Connect message as returned by intent endpoints. */
export interface IntentMessage {
  address: string;
  amount: string;
  payload?: string;
  stateInit?: string;
}

export interface Intent {
  listing_id: number;
  sale_address: Addr;
  valid_until: number;
  messages: IntentMessage[];
}

export interface AuthPayload {
  payload: string;
}

export interface AuthToken {
  token: string;
  address: Addr;
}

export interface TonProofBody {
  address: string;
  network: string;
  public_key: string;
  proof: {
    timestamp: number;
    domain: { lengthBytes: number; value: string };
    signature: string;
    payload: string;
  };
  state_init: string;
}

export interface RegisterTxBody {
  boc: string;
  type: Extract<TxType, 'list' | 'buy' | 'cancel'>;
  nft_address: string;
  listing_id?: number;
}

/**
 * Realtime event relayed by the API from the indexer (Postgres LISTEN/NOTIFY → WebSocket).
 * Addresses here are raw ("0:hex"); the UI only uses them to invalidate caches.
 */
export interface WsEvent {
  type: 'hello' | 'listing.updated' | 'tx.updated';
  nft_address?: string;
  listing_id?: number;
  tx_hash?: string;
  status: string;
  at?: string;
}
