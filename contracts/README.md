# contracts — Blueprint project

Standard Getgems contracts (TEP-62/64), vendored unmodified from
[getgems-io/nft-contracts](https://github.com/getgems-io/nft-contracts) (MIT, see `contracts/LICENSE-getgems`):

| Contract | Role |
|---|---|
| `nft-collection-editable.fc` | TEP-62 collection, off-chain TEP-64 metadata, mint / batch mint |
| `nft-item.fc` | TEP-62 item, `transfer` / `ownership_assigned` |
| `nft-fixprice-sale-v3r3.fc` | fixed-price sale; deployable only by the marketplace or the NFT |
| `nft-marketplace-v2.fc` | deploys signed sale StateInits (`op=1`), key held by the backend |

Our code: TypeScript wrappers (`wrappers/`), deploy scripts (`scripts/`), sandbox tests (`tests/`).

```bash
npm ci
npx blueprint build --all           # → build/*.compiled.json
npm test                            # @ton/sandbox: list → buy, cancel, negative cases
npm run export:code                 # sync compiled code into ../internal/chain/ton/codes
npm run deploy:marketplace          # testnet, TON Connect; prints MARKETPLACE_* for .env
METADATA_BASE_URL=https://your.domain ITEMS_COUNT=8 npm run deploy:collection
COLLECTION_ADDRESS=EQ... npx blueprint run mintItem --testnet --tonconnect
```

Message shapes used by the backend (`internal/chain/ton`) mirror these wrappers byte-for-byte; the
sandbox test `tests/marketplace.spec.ts` is the executable specification of the listing / buy / cancel flow.
