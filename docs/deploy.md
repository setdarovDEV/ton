# Deployment checklist (testnet demo on a VPS)

## 0. Prerequisites
- A testnet wallet with ≥ 3 TON (`@testgiver_ton_bot`), Tonkeeper or MyTonWallet switched to testnet.
- A tonapi key from https://tonconsole.com (free tier is enough).
- A domain pointing at the VPS; Docker 24+ with compose plugin.

## 1. Contracts (from your laptop, once)
```bash
cd contracts && npm ci
npm run deploy:marketplace            # TON Connect QR → sign with your testnet wallet
# copy MARKETPLACE_ADDRESS / MARKETPLACE_PRIVATE_KEY into .env on the server
METADATA_BASE_URL=https://tonnet.example.com ITEMS_COUNT=8 npm run deploy:collection
# copy COLLECTIONS=... into .env
```
Metadata JSON lives in `web/public/metadata/` and is served by the web container at
`https://$DOMAIN/metadata/...`, which is what the on-chain content URLs point to.

## 2. Server
```bash
git clone <repo> tonnet && cd tonnet
cp .env.example .env && $EDITOR .env
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build
docker compose logs -f indexer       # watch the first full sync
```

## 3. Smoke test
1. Open `https://$DOMAIN`, connect Tonkeeper (testnet). The wallet shows the ton_proof request.
2. `My NFTs` lists the minted items. `Sell` one for 1 TON → wallet shows two messages.
3. After ~5 s the item appears in `Explore` as active.
4. Buy it from a second wallet → "pending" → "confirmed", owner updated.

## 4. Operations
- Rotate `JWT_SECRET` to invalidate all sessions.
- `docker compose exec postgres psql -U tonnet` for ad-hoc queries.
- Backups: `docker compose exec postgres pg_dump -U tonnet tonnet > backup.sql`.
