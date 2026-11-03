-- Seed Vault packs: one row per pack added to a strain's seeds (the first
-- one and every restock), with what it cost. price_cents is NULL when no
-- price was entered; free marks a freebie (in Value, not in Cost). used_up
-- is set on a strain's packs when its seeds are restocked from zero, so the
-- Seed Vault counts only the packs its seeds still come from. Rows are
-- deleted with their strain in code (production SQLite doesn't cascade).
CREATE TABLE IF NOT EXISTS strain_pack (
    id SERIAL PRIMARY KEY,
    strain_id INTEGER NOT NULL,
    seeds INTEGER NOT NULL DEFAULT 0,
    price_cents INTEGER,
    free INTEGER NOT NULL DEFAULT 0,
    added_on TEXT NOT NULL,
    used_up INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_strain_pack_strain ON strain_pack (strain_id);
