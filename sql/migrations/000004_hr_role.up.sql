-- Add the HR role. HR is organization-wide (approves anyone, sees the full
-- directory) and, for now, mirrors admin's access. Kept as its own role so
-- admin-only powers can diverge later without touching this constraint again.
--
-- The role column's CHECK is a named constraint (employees_role_check, the
-- inline default name), so widening the allowed set is a drop + re-add.
ALTER TABLE employees DROP CONSTRAINT employees_role_check;
ALTER TABLE employees ADD CONSTRAINT employees_role_check
    CHECK (role IN ('employee', 'manager', 'admin', 'hr'));
