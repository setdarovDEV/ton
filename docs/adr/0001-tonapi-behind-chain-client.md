# ADR-0001: Read chain data through tonapi.io behind a `ChainClient` interface

**Status:** accepted · **Date:** 2026-11-06

## Context
The indexer needs collection data, item owners, account transactions, get-method results and
public keys. Options are a liteclient (tonutils-go, direct to lite servers) or an indexed HTTP API
(tonapi.io / toncenter). The client also asked for a "flexible architecture, additional chains".

## Decision
`internal/chain` defines a chain-agnostic `Client` interface (collections, items, transactions,
get-methods, public key). `internal/chain/ton` implements it over tonapi v2 REST with a token-bucket
rate limiter, per-call context timeouts and wrapped errors. Nothing outside `internal/chain/ton`
imports tonapi types.

## Alternatives
- **Liteclient (tonutils-go)** — no third-party dependency, but requires parsing raw cells for every
  contract type, running get-methods ourselves and managing lite-server connectivity. Right choice
  for production, wrong for a 4-day demo.
- **toncenter v3** — comparable; tonapi has better NFT-specific endpoints (`/nfts/collections/{id}/items`).

## Consequences
- Fast delivery; indexer code is small and testable with a fake `Client`.
- We depend on tonapi availability and rate limits (1 rps without a key). Mitigated by the limiter,
  an API key, and the fact that the API process never calls the chain — only the indexer does.
- Swapping to a liteclient is a new implementation of `chain.Client`; the domain and indexer do not change.
