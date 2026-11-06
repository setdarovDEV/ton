# tonnet — TON NFT Marketplace (demo)

[![CI](https://github.com/setdarovDEV/tonnet/actions/workflows/ci.yml/badge.svg)](https://github.com/setdarovDEV/tonnet/actions/workflows/ci.yml)

A small, complete, working vertical slice of a Getgems-style, TON-first NFT marketplace on **testnet**:
**wallet login (TON Connect 2 + `ton_proof`) → collection indexing → fixed-price listing → buy → on-chain confirmation**.
Go + PostgreSQL backend with a separate indexer, React frontend, standard Getgems contracts deployed with Blueprint,
one-command Docker Compose, CI, tests and the architecture decisions written down.

> **Demo links** — Live: `https://<DOMAIN>` · Walkthrough video (3 min): `<LOOM-URL>` · Testnet collection: `<EXPLORER-URL>`

---

## Architecture

```mermaid
flowchart LR
    W[Tonkeeper / MyTonWallet<br/>testnet] -- TON Connect 2<br/>ton_proof --> SPA[React SPA<br/>Vite · TanStack Query · Tailwind]
    SPA -- REST + WS --> API[Go API<br/>cmd/api]
    API -- reads only --> PG[(PostgreSQL 16)]
    IDX[Go Indexer<br/>cmd/indexer] -- upserts + pg_notify --> PG
    IDX -- ChainClient --> TONAPI[tonapi.io testnet]
    TONAPI --- CHAIN[TON testnet<br/>collection · items · marketplace · sale contracts]
    W -- signed messages --> CHAIN
```

### Core principle: **the DB is the source of truth for the UI; the chain is the source of truth for ownership**

- The **API never calls the chain.** It serves PostgreSQL only — fast, paginated, sortable.
- The **indexer is the only writer** of chain-derived state. Every 3–5 s it reads collections, items and
  transactions through a `ChainClient` interface and upserts them.
- A user action creates a **pending** record immediately; only an observed on-chain owner change promotes it:

  | Action | What the wallet sends | Pending state | Indexer observes | Final state |
  |---|---|---|---|---|
  | List | signed sale deploy via marketplace **+** NFT transfer → sale | `listing.pending`, `tx.pending` | `nft.owner == sale_contract` | `listing.active`, `tx.confirmed` |
  | Buy | `price + gas` → sale contract | `tx.pending` | `nft.owner == buyer` | `listing.sold`, `tx.confirmed` |
  | Cancel | `op=3` → sale contract | `tx.pending` | `nft.owner == seller` | `listing.cancelled`, `tx.confirmed` |

  Changes are pushed to the browser via PostgreSQL `LISTEN/NOTIFY` → WebSocket, with REST polling as the fallback.

Full diagrams and the module map: [docs/architecture.md](docs/architecture.md).

## How to run

### Prerequisites

- Docker 24+ with the Compose plugin (that is all you need for the default path)
- For development without Docker: Go 1.25+, Node.js 22+, PostgreSQL 16

### 1. Run everything with Docker Compose

```bash
git clone https://github.com/setdarovDEV/tonnet.git
cd tonnet
cp .env.example .env
```

Open `.env` and set at least these values:

| Variable | What to put |
|---|---|
| `JWT_SECRET` | any random string, 16+ characters |
| `MARKETPLACE_ADDRESS` | the marketplace contract address (see step 3). For read-only browsing before you deploy, any testnet address works |
| `MARKETPLACE_PRIVATE_KEY` | 32-byte hex seed printed by `deploy:marketplace`. For read-only browsing: `openssl rand -hex 32` |
| `COLLECTIONS` | comma-separated collection addresses to index (your own from step 3, or any testnet TEP-62 collection, e.g. `0:bc0c37a1079f0d19c0ee3afc363437f1c33aae6eb6ec1466c424deeac5194a90`) |
| `TONAPI_KEY` | optional; get one at <https://tonconsole.com>. Without it tonapi allows ~1 request/second |

Then start the stack:

```bash
docker compose up --build
```

Open <http://localhost:3000>. Services:

| Service | Port | Role |
|---|---|---|
| `web` | 3000 | React SPA served by nginx; proxies `/api` and `/ws` to the API |
| `api` | 8080 (`API_PORT`) | REST + WebSocket, read-only on the DB |
| `indexer` | — | polls TON testnet, writes the DB |
| `postgres` | 5432 (`POSTGRES_PORT`) | PostgreSQL 16 |

If port 8080 is already used on your machine, set `API_PORT=18080` in `.env`; the web container talks to the API internally, so nothing else changes.

Check it works:

```bash
curl http://localhost:3000/healthz
curl http://localhost:3000/api/collections
docker compose logs -f indexer     # "owner change" lines appear as items are indexed
```

Stop with `docker compose down` (add `-v` to drop the database volume).

### 2. Use the marketplace

1. Install Tonkeeper (or MyTonWallet) and switch it to **testnet**; get test TON from [@testgiver_ton_bot](https://t.me/testgiver_ton_bot).
2. Click **Connect Wallet** in the header. The wallet signs a `ton_proof`; the backend verifies it and issues a JWT (green check next to the button).
3. **My NFTs** shows what the connected wallet owns. Press **Sell**, enter a price, confirm the transaction in the wallet (two messages: deploy sale contract + transfer NFT to it). The listing shows `pending` until the indexer sees the NFT owned by the sale contract, then `active`.
4. **Explore** lists active sales. Press **Buy** with another wallet: you pay `price + 0.15 TON`; unused gas is refunded by the contract. The status goes `pending → confirmed` and the NFT appears in the buyer's My NFTs.
5. The seller can press **Cancel** on an active listing to get the NFT back.

Note: wallets require the TON Connect manifest to be served over HTTPS, so connecting a wallet works on a deployed domain (see `docs/deploy.md`), not on plain `http://localhost`. Browsing, indexing and the API work locally without a wallet.

### 3. Deploy the contracts to testnet (once)

```bash
cd contracts
npm install
npm run build                    # compiles the FunC sources with Blueprint
npm run deploy:marketplace       # prints MARKETPLACE_ADDRESS and MARKETPLACE_PRIVATE_KEY
METADATA_BASE_URL=https://your-domain.com npm run deploy:collection   # prints COLLECTIONS=...
```

Both scripts ask you to confirm the transaction through TON Connect (scan the QR with a testnet wallet that has a few TON). Put the printed values into `.env` and restart: `docker compose up --build -d`.

### 4. Development mode (without Docker)

```bash
docker compose up -d postgres                      # or any local PostgreSQL 16
export DATABASE_URL=postgres://tonnet:tonnet@localhost:5432/tonnet?sslmode=disable
go run ./cmd/api                                   # http://localhost:8080, runs migrations on start
go run ./cmd/indexer                               # separate process
cd web && npm install && npm run dev               # http://localhost:5173, proxies /api to :8080
```

Useful targets: `make test`, `make test-integration` (testcontainers), `make lint`; `cd contracts && npm test` (Sandbox tests).

## Contracts (standard Getgems, deployed with Blueprint)

This project **uses the standard, open-source Getgems contracts (MIT), not custom-written ones**:
`nft-collection-editable`, `nft-item` (TEP-62/64), `nft-fixprice-sale-v3r3` and `nft-marketplace-v2`,
vendored unmodified in [`contracts/contracts/`](contracts/contracts). Our code is the wrappers, scripts and
sandbox tests ([`contracts/`](contracts/README.md)).

```bash
cd contracts && npm ci && npx blueprint build --all && npm test
npm run deploy:marketplace                                  # → MARKETPLACE_ADDRESS, MARKETPLACE_PRIVATE_KEY
METADATA_BASE_URL=https://<DOMAIN> ITEMS_COUNT=8 npm run deploy:collection   # → COLLECTIONS
```

Sales are deployed **through the marketplace contract**: the backend signs `hash(stateInit, body)` with the
marketplace key, the user's wallet sends that payload, the marketplace deploys the sale — the v3r3 sale accepts
deploys only from its `marketplace_address`. Item metadata (TEP-64 off-chain) is served from `web/public/metadata/`.

## API

| Method | Path | Auth | Description |
|---|---|---|---|
| `POST` | `/api/auth/payload` | — | random `ton_proof` payload (HMAC-signed, expires) |
| `POST` | `/api/auth/verify` | — | verifies `ton_proof` (signature, domain, timestamp, public key ↔ address) → JWT |
| `GET` | `/api/collections` | — | indexed collections (with `items_count`) |
| `GET` | `/api/collections/{addr}` | — | one collection |
| `GET` | `/api/collections/{addr}/items?sort=index\|price_asc\|price_desc&page=&limit=` | — | items with active listing price |
| `GET` | `/api/items/{addr}` | — | `{item, transactions}`: owner, listing, tx history |
| `GET` | `/api/users/{addr}/items` | — | My NFTs (owned directly or via own active sale) |
| `GET` | `/api/listings?status=active&sort=newest\|price_asc\|price_desc&page=` | — | Explore |
| `GET` | `/api/listings/{id}` | — | listing detail |
| `POST` | `/api/listings/intent` | JWT | `{nft_address, price_nano}` → sale address + 2 wallet messages; writes `pending` listing |
| `POST` | `/api/listings/{id}/buy-intent` | JWT | → 1 wallet message (`price + 0.15 TON`, `op=2`, `query_id=listing id`) |
| `POST` | `/api/listings/{id}/cancel-intent` | JWT (seller) | → 1 wallet message (`op=3`) |
| `POST` | `/api/transactions` | JWT | `{boc, type, nft_address, listing_id}` → registers pending tx, returns its hash |
| `WS` | `/ws` | — | `listing.updated` / `tx.updated` events relayed from the indexer via Postgres `LISTEN/NOTIFY` |
| `GET` | `/api/transactions/{hash}` | — | `pending \| confirmed \| failed` (polling) |

Addresses are accepted in raw (`0:hex`) or user-friendly form and returned as both `raw` and `address`
(user-friendly, bounceable, testnet flag). Errors: `{"error":{"code":"…","message":"…"}}`.

## Data model

```
collections(address PK, owner, name, description, image_url, metadata jsonb, next_item_index, indexed_at)
nft_items(address PK, collection_address FK, item_index, owner, name, image_url, metadata jsonb, updated_at)
listings(id PK, nft_address FK, sale_contract UNIQUE, seller, price_nano, status ∈ {pending,active,sold,cancelled}, created_at, updated_at)
transactions(hash PK, type ∈ {list,buy,cancel,transfer,mint}, nft_address, from_addr, to_addr, amount_nano, lt, status ∈ {pending,confirmed,failed}, seen_at)
indexer_cursor(key PK, last_lt)
```
Indexes: `nft_items(owner)`, `nft_items(collection_address)`, `listings(status, price_nano)`, `transactions(nft_address, lt)`.
Migrations: golang-migrate, embedded, applied on start.

## Indexer design

- **Incremental**: per-collection cursor (`indexer_cursor.last_lt`) over account transactions; items are
  paged and diffed against the DB to detect owner changes (`OwnerChanged` events).
- **Idempotent**: all writes are upserts keyed by address / tx hash; listing transitions go through a
  state machine (`pending→active→sold|cancelled`) that rejects invalid moves, so replaying a batch is a no-op.
- **Reconcile**: every `INDEXER_INTERVAL` owners are re-read from the collection item list (reconcile), and every `ITEM_SCAN_INTERVAL` each item's transactions are scanned for history.
- **Tx tracker**: pending transactions are confirmed by the resulting owner change, and marked `failed`
  after `TX_PENDING_TTL` if nothing changed.
- **Rate-limited**: token bucket (`TONAPI_RPS`), `context` timeout on every chain and DB call,
  errors wrapped with `%w`, structured `slog` logging.

## TON specifics worth checking

- **`ton_proof`**: the backend rebuilds `ton-proof-item-v2/` ‖ workchain ‖ address-hash ‖ domain-len ‖ domain ‖ timestamp ‖ payload,
  verifies `ed25519(sha256(0xffff ‖ "ton-connect" ‖ sha256(msg)))` with the wallet public key, checks the domain against
  `TON_PROOF_DOMAINS`, the timestamp window and that the public key matches the address (via `state_init` hash, or chain
  lookup for deployed wallets). Only then is a JWT issued. We never trust a bare address.
- **Address formats**: stored raw, compared raw, displayed user-friendly with the testnet flag; bounceable for contracts.
- **Gas and fees in buy**: the sale requires `value ≥ price + 0.1 TON` (`min_gas_amount`). We send `price + 0.15 TON`.
  The contract pays seller `price − marketplace_fee − royalty`, pays fee and royalty, then transfers the NFT with
  mode 128 so all remaining value goes to the NFT contract, which forwards `0.03 TON` `ownership_assigned` to the buyer and
  returns the rest as `excesses` to `response_destination` (the buyer). Net cost to the buyer ≈ price + a few hundredths of a TON.
- **Listing**: two messages in one TON Connect transaction — marketplace deploy (0.05 TON) and NFT transfer
  (0.1 TON, `forward_amount` 0.05 TON so `ownership_assigned` reaches the sale and initializes it).
- **Royalty** is read from the collection (`royalty_params`) and embedded in the sale; we do not compute statistics on it.

## Architecture Decision Records

| ADR | Decision |
|---|---|
| [0001](docs/adr/0001-tonapi-behind-chain-client.md) | tonapi.io behind a `ChainClient` interface (liteclient later, same interface) |
| [0002](docs/adr/0002-db-source-of-truth-for-ui.md) | DB is source of truth for UI; chain for ownership; pending → confirmed via indexer |
| [0003](docs/adr/0003-standard-getgems-contracts.md) | Standard Getgems contracts; sales deployed through the marketplace with a backend-signed payload |
| [0004](docs/adr/0004-polling-plus-listen-notify.md) | Poll the chain; push to UI with `LISTEN/NOTIFY` → WebSocket; REST polling as fallback |
| [0005](docs/adr/0005-separate-indexer-and-api-processes.md) | Indexer and API are separate processes sharing one module |

## Multi-chain readiness

`internal/chain.Client` is the only thing the indexer and intent builders talk to. Adding a chain means:
1. implement `chain.Client` in `internal/chain/<name>` (collections, items, transactions, get-methods, public key);
2. provide an address normalizer for that chain's formats;
3. register it in `cmd/*` by `CHAIN=<name>`.
`internal/domain` and `internal/http` do not change. Jettons (TEP-74) as a payment currency would follow the same path.

## Testing

- **Go unit**: indexer parsing + state machine (`internal/domain`, `internal/indexer`, coverage gate ≥ 70% in CI).
- **Go integration**: API + repo against PostgreSQL via testcontainers (`go test -tags integration ./...`).
- **Contracts**: `@ton/sandbox` tests execute the real bytecode: list → buy, cancel, under-paid buy, foreign deploy, bad signature.
- **CI**: `golangci-lint`, `go vet`, tests with `-race`, web build, Blueprint build + sandbox tests, `docker compose build`.

## Out of scope / Roadmap

Auctions, offers and bids · royalty accounting (read only today) · multi-chain (abstraction in place, one implementation) ·
admin panel, moderation, reports · mainnet deployment and audit · indexer leader election for HA · liteclient `ChainClient`.

## Known limitations

- Depends on tonapi.io availability and rate limits (1 rps without a key); the indexer backs off but cannot go faster.
- Listing sends two messages in one transaction; if the NFT transfer were processed before the sale deploy, the
  `ownership_assigned` would bounce and the NFT would sit on an uninitialized sale. Getgems takes the same approach; the
  marketplace `op=555` emergency path can recover it, which we do not expose in the UI.
- No block re-org handling (acceptable on TON at this depth; owners are re-read on reconcile anyway).
- Confirmation latency 3–10 s (poll interval + chain finality).
- JWT is kept in `localStorage`; fine for a demo, would be an httpOnly cookie in production.
- Testnet only; no mainnet config was exercised.

## Links

[TEP-62 NFT standard](https://github.com/ton-blockchain/TEPs/blob/master/text/0062-nft-standard.md) ·
[TEP-64 token data](https://github.com/ton-blockchain/TEPs/blob/master/text/0064-token-data-standard.md) ·
[TEP-74 jettons](https://github.com/ton-blockchain/TEPs/blob/master/text/0074-jettons-standard.md) ·
[TON Connect](https://docs.ton.org/develop/dapps/ton-connect) ·
[Getgems contracts](https://github.com/getgems-io/nft-contracts) ·
[tonutils-go](https://github.com/xssnick/tonutils-go) ·
[Blueprint](https://github.com/ton-org/blueprint) ·
[tonapi testnet](https://testnet.tonapi.io) ·
[testnet faucet](https://t.me/testgiver_ton_bot)

License: MIT (ours) · Getgems contracts MIT (see [contracts/contracts/LICENSE-getgems](contracts/contracts/LICENSE-getgems)).
