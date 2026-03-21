-- =============================================================================
-- Migration: 000001_init_schema  (UP)
-- Description: Creates the initial database schema for ArthaLedger.
--              Tables: users, accounts, categories, transactions, budgets, alerts
-- =============================================================================

-- ─────────────────────────────────────────────────────────────────────────────
-- USERS
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    email       VARCHAR(255) NOT NULL UNIQUE,
    password    VARCHAR(255) NOT NULL,               -- bcrypt hash
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ                          -- soft-delete support (GORM)
);

CREATE INDEX IF NOT EXISTS idx_users_email      ON users (email);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);

-- ─────────────────────────────────────────────────────────────────────────────
-- ACCOUNTS  (bank, cash, credit card, investment, …)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS accounts (
    id          BIGSERIAL    PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    type        VARCHAR(50)  NOT NULL,               -- 'bank' | 'cash' | 'credit_card' | 'investment'
    balance     NUMERIC(15,2) NOT NULL DEFAULT 0.00,
    currency    VARCHAR(10)  NOT NULL DEFAULT 'INR',
    is_active   BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_accounts_user_id    ON accounts (user_id);
CREATE INDEX IF NOT EXISTS idx_accounts_deleted_at ON accounts (deleted_at);

-- ─────────────────────────────────────────────────────────────────────────────
-- CATEGORIES  (system-wide when user_id IS NULL, per-user otherwise)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS categories (
    id          BIGSERIAL    PRIMARY KEY,
    user_id     BIGINT       REFERENCES users (id) ON DELETE CASCADE,  -- NULL = system category
    name        VARCHAR(100) NOT NULL,
    type        VARCHAR(20)  NOT NULL,               -- 'income' | 'expense'
    icon        VARCHAR(50),
    color       VARCHAR(20),
    is_system   BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_categories_user_id ON categories (user_id);
CREATE INDEX IF NOT EXISTS idx_categories_type    ON categories (type);

-- ─────────────────────────────────────────────────────────────────────────────
-- TRANSACTIONS
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS transactions (
    id           BIGSERIAL     PRIMARY KEY,
    user_id      BIGINT        NOT NULL REFERENCES users    (id) ON DELETE CASCADE,
    account_id   BIGINT        NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    category_id  BIGINT        REFERENCES categories (id)  ON DELETE SET NULL,
    amount       NUMERIC(15,2) NOT NULL,
    type         VARCHAR(20)   NOT NULL,              -- 'income' | 'expense' | 'transfer'
    description  TEXT,
    note         TEXT,
    date         DATE          NOT NULL,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_transactions_user_id     ON transactions (user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_account_id  ON transactions (account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_category_id ON transactions (category_id);
CREATE INDEX IF NOT EXISTS idx_transactions_date        ON transactions (date);
CREATE INDEX IF NOT EXISTS idx_transactions_deleted_at  ON transactions (deleted_at);

-- ─────────────────────────────────────────────────────────────────────────────
-- BUDGETS
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS budgets (
    id           BIGSERIAL     PRIMARY KEY,
    user_id      BIGINT        NOT NULL REFERENCES users      (id) ON DELETE CASCADE,
    category_id  BIGINT        REFERENCES categories (id)     ON DELETE SET NULL,
    name         VARCHAR(255)  NOT NULL,
    amount       NUMERIC(15,2) NOT NULL,
    period       VARCHAR(20)   NOT NULL,              -- 'daily' | 'weekly' | 'monthly' | 'yearly'
    start_date   DATE          NOT NULL,
    end_date     DATE,                                -- NULL = open-ended
    is_active    BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_budgets_user_id    ON budgets (user_id);
CREATE INDEX IF NOT EXISTS idx_budgets_deleted_at ON budgets (deleted_at);

-- ─────────────────────────────────────────────────────────────────────────────
-- ALERTS  (in-app notifications)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS alerts (
    id          BIGSERIAL    PRIMARY KEY,
    user_id     BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type        VARCHAR(50)  NOT NULL,               -- 'budget_exceeded' | 'unusual_spending' | …
    title       VARCHAR(255) NOT NULL,
    message     TEXT         NOT NULL,
    is_read     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alerts_user_id ON alerts (user_id);
CREATE INDEX IF NOT EXISTS idx_alerts_is_read ON alerts (is_read);

-- ─────────────────────────────────────────────────────────────────────────────
-- SEED: Default system categories
-- ─────────────────────────────────────────────────────────────────────────────
INSERT INTO categories (user_id, name, type, is_system, created_at, updated_at) VALUES
    -- Income
    (NULL, 'Salary',             'income',  TRUE, NOW(), NOW()),
    (NULL, 'Business',           'income',  TRUE, NOW(), NOW()),
    (NULL, 'Investment Returns', 'income',  TRUE, NOW(), NOW()),
    (NULL, 'Freelance',          'income',  TRUE, NOW(), NOW()),
    (NULL, 'Other Income',       'income',  TRUE, NOW(), NOW()),
    -- Expense
    (NULL, 'Food & Dining',      'expense', TRUE, NOW(), NOW()),
    (NULL, 'Transportation',     'expense', TRUE, NOW(), NOW()),
    (NULL, 'Shopping',           'expense', TRUE, NOW(), NOW()),
    (NULL, 'Entertainment',      'expense', TRUE, NOW(), NOW()),
    (NULL, 'Bills & Utilities',  'expense', TRUE, NOW(), NOW()),
    (NULL, 'Healthcare',         'expense', TRUE, NOW(), NOW()),
    (NULL, 'Education',          'expense', TRUE, NOW(), NOW()),
    (NULL, 'Travel',             'expense', TRUE, NOW(), NOW()),
    (NULL, 'Housing',            'expense', TRUE, NOW(), NOW()),
    (NULL, 'Personal Care',      'expense', TRUE, NOW(), NOW()),
    (NULL, 'Others',             'expense', TRUE, NOW(), NOW());
