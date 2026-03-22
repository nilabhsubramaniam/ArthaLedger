-- =============================================================================
-- Migration: 000003_add_categorization_rules (DOWN)
-- Description: Reverses the UP migration by dropping the rules table.
-- =============================================================================

DROP TABLE IF EXISTS categorization_rules;
