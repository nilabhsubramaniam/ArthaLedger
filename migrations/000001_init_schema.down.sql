-- =============================================================================
-- Migration: 000001_init_schema  (DOWN)
-- Description: Drops all tables created by the UP migration.
--              Order matters: children before parents (FK constraints).
-- =============================================================================

DROP TABLE IF EXISTS alerts;
DROP TABLE IF EXISTS budgets;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS users;
