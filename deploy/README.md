# Deploying the demo to a VPS (Docker Compose + Caddy)

1. Point a DNS A record (e.g. `tonnet.example.com`) at the VPS.
2. On the VPS (Docker 24+ with the compose plugin):

   ```bash
   git clone <repo-url> tonnet && cd tonnet
   cp .env.example .env
   # edit .env: DOMAIN, JWT_SECRET, TONAPI_KEY, MARKETPLACE_*, COLLECTIONS, TON_PROOF_DOMAINS
   docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build
   ```

3. Caddy obtains a certificate for `$DOMAIN` automatically. The SPA is served at `https://$DOMAIN`,
   the API at `https://$DOMAIN/api`, WebSocket at `wss://$DOMAIN/ws`.
4. Contracts: the TON Connect manifest (`web/public/tonconnect-manifest.json`) and the NFT metadata
   (`web/public/metadata/*`) are served from the same domain. Deploy the collection with
   `METADATA_BASE_URL=https://$DOMAIN` so that the on-chain content URLs resolve.
5. Update: `git pull && docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build`.
6. Logs: `docker compose logs -f api indexer`.

See [docs/deploy.md](../docs/deploy.md) for the full checklist.
