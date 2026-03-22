-- =============================================================================
-- Migration: 000003_add_categorization_rules (UP)
-- Description: Adds the categorization_rules table.
--              Each rule maps a keyword to a category for a specific user.
--              When a transaction is created without an explicit category_id,
--              the rules engine scans this table to auto-assign one.
-- =============================================================================

CREATE TABLE IF NOT EXISTS categorization_rules (
    id          BIGSERIAL    PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES users      (id) ON DELETE CASCADE,
    category_id BIGINT       NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    keyword     VARCHAR(100) NOT NULL,   -- case-insensitive substring match against description
    priority    INT          NOT NULL DEFAULT 0,  -- higher priority wins when multiple rules match
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Fast lookup by user (every transaction create triggers a lookup)
CREATE INDEX IF NOT EXISTS idx_cat_rules_user_id ON categorization_rules (user_id);

-- Unique constraint: one keyword per category per user (prevents duplicates)
CREATE UNIQUE INDEX IF NOT EXISTS idx_cat_rules_user_keyword
    ON categorization_rules (user_id, lower(keyword));
