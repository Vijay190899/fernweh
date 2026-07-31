CREATE TABLE IF NOT EXISTS listings (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL,
    category              TEXT NOT NULL,  -- beach|city|ski|wellness|adventure|countryside
    destination           TEXT NOT NULL,
    country               TEXT NOT NULL,
    price_per_night_cents INTEGER NOT NULL,
    currency              TEXT NOT NULL DEFAULT 'EUR',
    rating                NUMERIC(2,1) NOT NULL,
    review_count          INTEGER NOT NULL DEFAULT 0,
    amenities             JSONB NOT NULL DEFAULT '[]',
    vibe_tags             JSONB NOT NULL DEFAULT '[]',
    description           TEXT,
    image_url             TEXT,
    months_best           JSONB NOT NULL DEFAULT '[]',  -- array of 1..12
    margin_tier           TEXT NOT NULL DEFAULT 'standard',  -- standard|preferred|premium
    content_status        TEXT NOT NULL DEFAULT 'complete',  -- complete|needs_enrichment|enriching|enriched|failed
    content_hash          TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_listings_category    ON listings (category);
CREATE INDEX IF NOT EXISTS idx_listings_country     ON listings (country);
CREATE INDEX IF NOT EXISTS idx_listings_destination ON listings (destination);
CREATE INDEX IF NOT EXISTS idx_listings_price       ON listings (price_per_night_cents);
CREATE INDEX IF NOT EXISTS idx_listings_status      ON listings (content_status);
CREATE INDEX IF NOT EXISTS idx_listings_amenities   ON listings USING GIN (amenities);
CREATE INDEX IF NOT EXISTS idx_listings_vibes       ON listings USING GIN (vibe_tags);

CREATE TABLE IF NOT EXISTS promotions (
    id         TEXT PRIMARY KEY,
    listing_id TEXT NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    label      TEXT NOT NULL,
    boost      DOUBLE PRECISION NOT NULL DEFAULT 0.1,  -- bounded score boost
    active     BOOLEAN NOT NULL DEFAULT true
);

CREATE INDEX IF NOT EXISTS idx_promotions_listing ON promotions (listing_id) WHERE active;

CREATE TABLE IF NOT EXISTS enrichment_audit (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    listing_id TEXT NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    field      TEXT NOT NULL,
    before     TEXT,
    after      TEXT NOT NULL,
    source     TEXT NOT NULL,  -- ai|template
    model      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_listing ON enrichment_audit (listing_id, created_at DESC);
