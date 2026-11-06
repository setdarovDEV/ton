# ADR-0004: Poll the chain; push to the UI with PostgreSQL LISTEN/NOTIFY over WebSocket

**Status:** accepted · **Date:** 2026-11-06

## Context
The UI should move from "pending" to "confirmed" without a reload. The indexer and the API are
separate processes, so the API cannot observe confirmations in memory.

## Decision
- Indexer → chain: **polling** every `INDEXER_INTERVAL` (4 s) plus a slower full reconcile.
  tonapi has no reliable push for arbitrary accounts and polling is trivially idempotent.
- Indexer → API: on every status change the indexer runs `pg_notify('tx_events', json)`.
  The API `LISTEN`s on the same channel and fans events out to `/ws` clients.
- Frontend: TanStack Query polls `GET /api/transactions/:hash` every 3 s **and** invalidates on WS events.
  Polling is the correctness path; WS is latency sugar.

## Alternatives
- **tonapi SSE/WebSocket streams** — exist, but reconnect/catch-up logic is more code than polling.
- **Redis pub/sub or a queue** — another service for a one-channel problem; Postgres already exists.
- **WS only, no polling** — a dropped socket would leave the UI stuck on "pending".

## Consequences
- Confirmation latency is 3–8 s on testnet; acceptable for a demo and documented.
- Adding API replicas is free: each one listens to the same channel.
