ALTER TABLE users
    DROP COLUMN IF EXISTS preferences,
    DROP COLUMN IF EXISTS must_change_password;
