-- Reverse the HR role. Any existing HR employees would violate the reverted
-- constraint, so demote them to admin (their closest equivalent) first.
UPDATE employees SET role = 'admin' WHERE role = 'hr';

ALTER TABLE employees DROP CONSTRAINT employees_role_check;
ALTER TABLE employees ADD CONSTRAINT employees_role_check
    CHECK (role IN ('employee', 'manager', 'admin'));
