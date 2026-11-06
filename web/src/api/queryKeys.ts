/** Centralised query keys so WebSocket events can invalidate precisely. */
export const keys = {
  collections: ['collections'] as const,
  collection: (addr: string) => ['collections', addr] as const,
  collectionItems: (addr: string, params: Record<string, unknown>) => ['collections', addr, 'items', params] as const,
  item: (addr: string) => ['items', addr] as const,
  userItems: (addr: string, params: Record<string, unknown>) => ['users', addr, 'items', params] as const,
  listings: (params: Record<string, unknown>) => ['listings', params] as const,
  tx: (hash: string) => ['tx', hash] as const,
};
