-- +goose Up

-- BRAND-001: one person posts for several products. brand is a free-text label
-- on an account ("Aaru", "Norviq"), so a client can show one product's accounts
-- at a time. It is a label, not a tenant: every account still belongs to one
-- user, and nothing is scoped by it. NULL means no brand.
ALTER TABLE accounts ADD COLUMN brand text;

-- +goose Down

ALTER TABLE accounts DROP COLUMN brand;
