import { qs, request } from './client';
import type {
  AuthPayload,
  AuthToken,
  Collection,
  Intent,
  Item,
  ItemDetail,
  Listing,
  ListingStatus,
  Page,
  RegisterTxBody,
  TonProofBody,
  Tx,
} from '@/lib/types';

export type ListingSort = 'price_asc' | 'price_desc' | 'newest';
export type ItemSort = 'index' | 'price_asc' | 'price_desc';

export const api = {
  auth: {
    payload: () => request<AuthPayload>('/api/auth/payload', { method: 'POST' }),
    verify: (body: TonProofBody) => request<AuthToken>('/api/auth/verify', { method: 'POST', body }),
  },
  collections: {
    list: (params: { page?: number; limit?: number } = {}) => request<Page<Collection>>(`/api/collections${qs(params)}`),
    get: (addr: string) => request<Collection>(`/api/collections/${encodeURIComponent(addr)}`),
    items: (addr: string, params: { page?: number; limit?: number; sort?: ItemSort }) =>
      request<Page<Item>>(`/api/collections/${encodeURIComponent(addr)}/items${qs(params)}`),
  },
  items: {
    get: (addr: string) => request<ItemDetail>(`/api/items/${encodeURIComponent(addr)}`),
  },
  users: {
    items: (addr: string, params: { page?: number; limit?: number } = {}) =>
      request<Page<Item>>(`/api/users/${encodeURIComponent(addr)}/items${qs(params)}`),
  },
  listings: {
    list: (params: { status?: ListingStatus; sort?: ListingSort; page?: number; limit?: number }) =>
      request<Page<Listing>>(`/api/listings${qs(params)}`),
    intent: (body: { nft_address: string; price_nano: string }) =>
      request<Intent>('/api/listings/intent', { method: 'POST', body, auth: true }),
    buyIntent: (id: number) => request<Intent>(`/api/listings/${id}/buy-intent`, { method: 'POST', auth: true }),
    cancelIntent: (id: number) =>
      request<Intent>(`/api/listings/${id}/cancel-intent`, { method: 'POST', auth: true }),
  },
  transactions: {
    register: (body: RegisterTxBody) => request<Tx>('/api/transactions', { method: 'POST', body, auth: true }),
    get: (hash: string) => request<Tx>(`/api/transactions/${encodeURIComponent(hash)}`),
  },
};
