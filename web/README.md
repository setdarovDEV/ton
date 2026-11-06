# web — TONNET React SPA

React 19 + Vite + TypeScript + Tailwind 4 + TanStack Query + `@tonconnect/ui-react`.

```sh
npm install
npm run dev      # http://localhost:5173, proxies /api and /ws to VITE_DEV_API_TARGET (default http://localhost:8080)
npm run build    # type-check + production bundle in dist/
npm run lint
```

Pages: Explore (`/`), Collections (`/collections`, `/collections/:addr`), Item (`/items/:addr`), My NFTs (`/me`).

Flows: ton_proof login → JWT (`features/auth`), list / buy / cancel via backend-built intents signed with
TON Connect `sendTransaction` (`features/listings`, `features/tx/useSendIntent`), tx tracking by polling
`GET /api/transactions/:hash` with optional WebSocket invalidation (`features/tx`).

`public/tonconnect-manifest.json` and `public/metadata/*` contain the placeholder domain
`https://tonnet.example.com` — replace it before deploying (see `public/metadata/README.md`).
