/**
 * Centralised query keys. Root keys are what WebSocket events and send flows invalidate;
 * leaf keys are what hooks subscribe with. Never spell a key literal outside this file.
 */
export const keys = {
  collectionsRoot: ['collections'] as const,
  collectionsList: (params: Record<string, unknown>) => ['collections', 'list', params] as const,
  collection: (addr: string) => ['collections', addr] as const,
  collectionItems: (addr: string, params: Record<string, unknown>) => ['collections', addr, 'items', params] as const,
  itemsRoot: ['items'] as const,
  item: (addr: string) => ['items', addr] as const,
  usersRoot: ['users'] as const,
  userItems: (addr: string, params: Record<string, unknown>) => ['users', addr, 'items', params] as const,
  listingsRoot: ['listings'] as const,
  listings: (params: Record<string, unknown>) => ['listings', params] as const,
  transactionsRoot: ['transactions'] as const,
  transaction: (hash: string) => ['transactions', hash] as const,
};
