# Off-chain metadata (TEP-64)

`collection.json` and `items/<index>.json` are the off-chain metadata referenced by the on-chain content cells
of the demo collection. nginx serves this folder at `/metadata/` with `application/json`.

The deploy script (`contracts/scripts/deployCollection.ts`) uses `METADATA_BASE_URL`:

- collection content → `${METADATA_BASE_URL}/metadata/collection.json`
- common content prefix → `${METADATA_BASE_URL}/metadata/items/`, item content → `<index>.json`

All `image` fields are absolute URLs. Before deploying, replace the placeholder domain with the one that will host the SPA:

```sh
# from the repo root
LC_ALL=C find web/public/metadata -type f -name '*.json' -exec sed -i '' 's#https://tonnet.example.com#https://your-domain.tld#g' {} +
```

(GNU sed: drop the `''` after `-i`.) Also update `web/public/tonconnect-manifest.json` the same way.
