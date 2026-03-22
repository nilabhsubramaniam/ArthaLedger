-- =============================================================================
-- Migration: 000002_add_transfer_reference  (DOWN)
-- Description: Removes transfer_reference_id from transactions.
-- =============================================================================

DROP INDEX IF EXISTS idx_transactions_transfer_ref;

ALTER TABLE transactions
    DROP COLUMN IF EXISTS transfer_reference_id;
