# Architecture

```mermaid
flowchart LR
    W[Tonkeeper / MyTonWallet<br/>testnet] -- TON Connect 2<br/>ton_proof --> SPA[React SPA<br/>Vite · TanStack Query]
    SPA -- REST + WS --> API[Go API<br/>cmd/api · chi]
    API -- read only --> PG[(PostgreSQL 16)]
    IDX[Go Indexer<br/>cmd/indexer] -- upsert · pg_notify --> PG
    IDX -- ChainClient --> TONAPI[tonapi.io<br/>testnet]
    TONAPI --- CHAIN[TON testnet<br/>collection · items · marketplace · sales]
    W -- signed messages --> CHAIN
    API -. LISTEN tx_events .-> PG
```

## Module layout (Go, clean architecture)

| Package | Responsibility | May import |
|---|---|---|
| `cmd/api`, `cmd/indexer` | wiring, config, graceful shutdown | everything below |
| `internal/http` | handlers, DTOs, validation, JWT middleware, WS hub | domain, repo (through interfaces), auth |
| `internal/indexer` | sync loop, reconcile, tx tracker | domain, chain, repo interfaces |
| `internal/domain` | entities, listing state machine, intent builders' rules | nothing internal |
| `internal/chain` | `Client` interface + chain-neutral types | — |
| `internal/chain/ton` | tonapi implementation, address normalization, cell builders, ton_proof | chain, tonutils-go |
| `internal/repo` | pgx repositories, migrations | domain |

Dependencies point inward; `internal/domain` has no imports from the other packages.

## Listing flow

```mermaid
sequenceDiagram
    actor U as User (wallet)
    participant FE as React SPA
    participant API as Go API
    participant MP as Marketplace contract
    participant S as Sale contract
    participant NFT as NFT item
    participant IDX as Indexer
    participant DB as PostgreSQL

    U->>FE: "Sell" + price
    FE->>API: POST /api/listings/intent {nft, price} (JWT)
    API->>API: build sale StateInit, sign hash(stateInit, body) with marketplace key
    API->>DB: INSERT listing status=pending, sale_contract=addr
    API-->>FE: {sale_address, messages:[deploy via MP, NFT transfer→sale]}
    FE->>U: tonConnectUI.sendTransaction(messages)
    U->>MP: op=1 + signature + stateInit + body (0.05 TON)
    MP->>S: deploy (sender == marketplace ⇒ accepted)
    U->>NFT: transfer new_owner=sale, forward 0.05 TON
    NFT->>S: ownership_assigned(prev_owner=seller)
    S->>S: nft_owner_address = seller ⇒ initialized
    FE->>API: POST /api/transactions {boc, type:list}
    API->>DB: INSERT transaction pending
    loop every 4 s
        IDX->>IDX: fetch items, diff owners
        IDX->>DB: nft.owner=sale ⇒ listing pending→active, tx→confirmed, pg_notify
    end
    DB-->>API: NOTIFY tx_events
    API-->>FE: WS event (+ polling GET /api/transactions/:hash)
```

## Buy flow

```mermaid
sequenceDiagram
    actor B as Buyer (wallet)
    participant FE as React SPA
    participant API as Go API
    participant S as Sale contract
    participant NFT as NFT item
    participant IDX as Indexer
    participant DB as PostgreSQL

    B->>FE: "Buy"
    FE->>API: POST /api/listings/{id}/buy-intent (JWT)
    API-->>FE: {address: sale, amount: price + 0.15 TON, payload: op=2 query_id=id}
    FE->>B: sendTransaction
    B->>S: price + gas
    S->>S: require value ≥ price + 0.1 TON
    S-->>Seller: price − fee − royalty
    S-->>Fee/Royalty: fee, royalty
    S->>NFT: transfer new_owner=buyer, response=buyer, mode 128 (all remaining)
    NFT-->>B: excesses (unused gas back)
    FE->>API: POST /api/transactions {boc, type:buy}
    IDX->>DB: nft.owner=buyer ⇒ listing active→sold, tx→confirmed
    DB-->>API: NOTIFY → WS → UI "Sold"
```

## Indexer loop

1. For every tracked collection: `GetCollection` → upsert metadata, `next_item_index`.
2. Page through `GetCollectionItems`; for every item whose owner differs from the DB → emit
   `OwnerChanged{nft, from, to}` and apply it through the domain state machine:
   - owner == `listings.sale_contract` (pending) → `active`
   - listing active and owner == seller → `cancelled`; any other owner → `sold`
   - matching pending `transactions` → `confirmed`.
3. `GetTransactions(collection, after_lt=cursor)` → record mint/transfer rows → advance `indexer_cursor`.
4. Pending transactions older than `TX_PENDING_TTL` with no owner change → `failed`.
5. Every `ITEM_SCAN_INTERVAL` scan each item's transactions for history; owners are reconciled from the collection item list on every round (safety net against missed changes).

Every step is idempotent: upserts keyed by address/hash; state transitions reject invalid moves
instead of double-applying them. Restarting the indexer mid-loop is safe.

## Address handling

- Stored and compared in **raw** form (`0:hex`, lowercase) — the only canonical form.
- Returned to clients as both `raw` and user-friendly `address` (base64url, bounceable, testnet flag on
  testnet). Wallet addresses are rendered non-bounceable in the UI where appropriate.
- Every inbound address is parsed with tonutils-go `address.ParseAddr`/`ParseRawAddr` and normalized.
