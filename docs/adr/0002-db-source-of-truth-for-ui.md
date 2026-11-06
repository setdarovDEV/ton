# ADR-0002: PostgreSQL is the source of truth for the UI; the chain is the source of truth for ownership

**Status:** accepted · **Date:** 2026-11-06

## Context
A marketplace UI must be fast and paginated (sort by price, filter by owner), while ownership and
sale state are decided on-chain and arrive with latency.

## Decision
The API reads **only** from PostgreSQL. The indexer is the single writer of chain-derived state.
User actions produce **pending** rows (listing `pending`, transaction `pending`) immediately; the
indexer flips them to `active`/`sold`/`cancelled` and `confirmed`/`failed` only after observing the
owner change on-chain. The frontend never trusts its own transaction as a success.

## Alternatives
- **API queries the chain on every request** — simple, but slow, rate-limited and impossible to sort/paginate.
- **Frontend marks the listing active after `sendTransaction` resolves** — a wallet accepting a
  transaction does not mean it executed; messages bounce, run out of gas or race.

## Consequences
- Reads are O(1) DB queries, horizontally scalable; the chain is hit at a bounded rate by one process.
- UI shows "pending" for 3–10 s until the indexer confirms. We make that state explicit.
- Idempotency is mandatory: the indexer may re-process the same transactions after a restart, so
  every write is an upsert and every status change goes through a guarded state machine.
