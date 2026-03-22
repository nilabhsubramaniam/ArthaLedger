-- =============================================================================
-- Migration: 000002_add_transfer_reference  (UP)
-- Description: Adds transfer_reference_id to transactions.
--
-- When a "transfer" transaction is created, two rows are inserted:
--   1. An expense on the source account
--   2. An income on the destination account
-- Both rows point to each other via transfer_reference_id so they can be
-- deleted / queried together as a single logical operation.
-- =============================================================================

ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS transfer_reference_id BIGINT
        REFERENCES transactions (id) ON DELETE SET NULL;

-- Index lets us quickly find the linked leg of a transfer pair
CREATE INDEX IF NOT EXISTS idx_transactions_transfer_ref
    ON transactions (transfer_reference_id);
