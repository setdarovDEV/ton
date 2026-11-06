# ADR-0005: Indexer and API are separate binaries sharing one Go module

**Status:** accepted · **Date:** 2026-11-06

## Context
Indexing is a background, rate-limited, stateful loop; serving HTTP is stateless and bursty. They
have different scaling and failure profiles.

## Decision
`cmd/api` and `cmd/indexer` are separate processes/containers that share `internal/domain`,
`internal/repo` and `internal/chain`. The indexer is a singleton (one writer, cursors in
`indexer_cursor`); the API is stateless and can run N replicas behind Caddy.

## Alternatives
- **One binary with goroutines** — fewer containers, but an API restart would reset indexer
  cursors mid-loop and you could not scale the API without duplicating the indexer.
- **Separate repositories** — would duplicate the domain model and the migrations.

## Consequences
- Clean architecture boundaries are enforced by the import graph (`cmd/*` → `internal/*`, never sideways).
- Docker Compose runs both; `docker compose up --scale api=3` works without changes.
- A second indexer instance would double-write; this is documented and left to a leader lock in the roadmap.
