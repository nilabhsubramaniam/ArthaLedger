-- =============================================================================
-- Migration: 000004_add_budget_alerts_phase3  (DOWN)
-- Description: Reverses the UP migration — removes budget_id column and index
--              from the alerts table.
-- =============================================================================

DROP INDEX IF EXISTS idx_alerts_budget_id;

ALTER TABLE alerts
    DROP COLUMN IF EXISTS budget_id;
