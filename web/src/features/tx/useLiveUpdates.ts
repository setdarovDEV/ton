import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { wsUrl } from '@/lib/config';
import type { WsEvent } from '@/lib/types';
import { keys } from '@/api/queryKeys';

/**
 * Optional WebSocket channel: the API relays indexer events (tx/listing status changes).
 * It only *invalidates* queries — polling remains the source of truth, so losing the socket is harmless.
 */
export function useLiveUpdates(): void {
  const qc = useQueryClient();

  useEffect(() => {
    let socket: WebSocket | null = null;
    let attempt = 0;
    let timer: number | undefined;
    let closed = false;

    const connect = () => {
      try {
        socket = new WebSocket(wsUrl);
      } catch {
        schedule();
        return;
      }
      socket.onopen = () => {
        attempt = 0;
      };
      socket.onmessage = (ev) => {
        let event: WsEvent;
        try {
          event = JSON.parse(String(ev.data)) as WsEvent;
        } catch {
          return;
        }
        if (event.type === 'tx.updated') {
          if (event.tx_hash) void qc.invalidateQueries({ queryKey: keys.transaction(event.tx_hash) });
          if (event.nft_address) void qc.invalidateQueries({ queryKey: keys.item(event.nft_address) });
        } else if (event.type === 'listing.updated') {
          void qc.invalidateQueries({ queryKey: keys.listingsRoot });
          if (event.nft_address) void qc.invalidateQueries({ queryKey: keys.item(event.nft_address) });
          void qc.invalidateQueries({ queryKey: keys.usersRoot });
          void qc.invalidateQueries({ queryKey: keys.collectionsRoot });
        }
      };
      socket.onclose = () => schedule();
      socket.onerror = () => socket?.close();
    };

    const schedule = () => {
      if (closed) return;
      const delay = Math.min(30_000, 1000 * 2 ** attempt++);
      timer = window.setTimeout(connect, delay);
    };

    connect();
    return () => {
      closed = true;
      if (timer) window.clearTimeout(timer);
      socket?.close();
    };
  }, [qc]);
}
