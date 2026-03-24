-- =============================================================================
-- Migration: 000004_add_budget_alerts_phase3  (UP)
-- Description: Adds budget_id foreign key to the alerts table so that each
--              alert can be traced back to the budget that triggered it.
--              This supports deduplication in the budget service (Phase 3).
-- =============================================================================

ALTER TABLE alerts
    ADD COLUMN IF NOT EXISTS budget_id BIGINT
        REFERENCES budgets (id) ON DELETE SET NULL;

-- Partial index: only index non-NULL budget_id values to keep the index small.
CREATE INDEX IF NOT EXISTS idx_alerts_budget_id
    ON alerts (budget_id)
    WHERE budget_id IS NOT NULL;
