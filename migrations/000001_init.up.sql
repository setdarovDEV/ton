-- Addresses are stored in raw form ("0:<64 hex>", lowercase). The API layer renders user-friendly forms.
CREATE TABLE IF NOT EXISTS collections (
    address          TEXT PRIMARY KEY,
    owner            TEXT,
    name             TEXT,
    description      TEXT,
    image_url        TEXT,
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
    next_item_index  BIGINT NOT NULL DEFAULT 0,
    indexed_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nft_items (
    address             TEXT PRIMARY KEY,
    collection_address  TEXT NOT NULL REFERENCES collections(address) ON DELETE CASCADE,
    item_index          BIGINT NOT NULL,
    owner               TEXT,
    name                TEXT,
    image_url           TEXT,
    metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS nft_items_owner_idx ON nft_items(owner);
CREATE INDEX IF NOT EXISTS nft_items_collection_idx ON nft_items(collection_address);

CREATE TABLE IF NOT EXISTS listings (
    id             BIGSERIAL PRIMARY KEY,
    nft_address    TEXT NOT NULL REFERENCES nft_items(address) ON DELETE CASCADE,
    sale_contract  TEXT NOT NULL UNIQUE,
    seller         TEXT NOT NULL,
    buyer          TEXT,
    price_nano     BIGINT NOT NULL CHECK (price_nano > 0),
    status         TEXT NOT NULL CHECK (status IN ('pending','active','sold','cancelled')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS listings_status_price_idx ON listings(status, price_nano);
CREATE INDEX IF NOT EXISTS listings_nft_idx ON listings(nft_address);

CREATE TABLE IF NOT EXISTS transactions (
    hash         TEXT PRIMARY KEY,
    type         TEXT NOT NULL CHECK (type IN ('list','buy','cancel','transfer','mint')),
    nft_address  TEXT,
    listing_id   BIGINT REFERENCES listings(id) ON DELETE SET NULL,
    from_addr    TEXT,
    to_addr      TEXT,
    amount_nano  BIGINT NOT NULL DEFAULT 0,
    lt           BIGINT NOT NULL DEFAULT 0,
    status       TEXT NOT NULL CHECK (status IN ('pending','confirmed','failed')),
    boc          TEXT,
    expires_at   TIMESTAMPTZ,
    seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS transactions_nft_lt_idx ON transactions(nft_address, lt);
CREATE INDEX IF NOT EXISTS transactions_status_idx ON transactions(status);

CREATE TABLE IF NOT EXISTS indexer_cursor (
    key      TEXT PRIMARY KEY,
    last_lt  BIGINT NOT NULL DEFAULT 0
);
