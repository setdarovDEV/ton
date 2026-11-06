# ---- build ----
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
# VITE_API_URL stays empty: nginx proxies /api and /ws to the api service (same-origin).
ARG VITE_API_URL=
ARG VITE_TON_NETWORK=testnet
ENV VITE_API_URL=$VITE_API_URL VITE_TON_NETWORK=$VITE_TON_NETWORK
RUN npm run build

# ---- serve ----
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
