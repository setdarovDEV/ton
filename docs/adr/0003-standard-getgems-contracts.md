# ADR-0003: Use the standard open-source Getgems contracts instead of writing our own

**Status:** accepted · **Date:** 2026-11-06

## Context
The demo needs a TEP-62 collection, TEP-62 items, a fixed-price sale contract and a way to deploy
sales. Writing FunC is slow and risky, and an unaudited custom sale contract is a liability.

## Decision
We vendor, unmodified, `nft-collection-editable.fc`, `nft-item.fc`, `nft-fixprice-sale-v3r3.fc` and
`nft-marketplace-v2.fc` from [getgems-io/nft-contracts](https://github.com/getgems-io/nft-contracts)
(MIT) and compile them with Blueprint. Our contribution is the TypeScript wrappers, deploy scripts,
sandbox tests and the Go code that builds the same cells server-side.

Sales are deployed **through the marketplace contract**: the backend signs
`hash(ref stateInit, ref body)` with the marketplace key; the user's wallet sends that signed payload
to the marketplace, which deploys the sale. This is exactly how Getgems works and it is the only
sender the v3r3 sale accepts at deploy time besides the NFT itself.

## Alternatives
- **Custom minimal sale contract** — smaller, but unaudited and not what the client runs in production.
- **`marketplace_address = seller`** — avoids the backend key, but then anyone can produce sales
  that claim to be ours and the fee address is unenforced.

## Consequences
- Battle-tested, audited bytecode; the client recognises the flow.
- The backend holds a signing key (`MARKETPLACE_PRIVATE_KEY`). It only authorizes sale deploys; it
  cannot move NFTs or funds, but it must still be kept secret.
- Honest attribution in the README: these contracts are not ours.
