-- INNER JOIN
-- name: GetUserWithDepartment :many
SELECT
    u.id,
    u.name,
    d.id as department_id,
    d.name as department_name
FROM users u
INNER JOIN departments d ON u.department_id = d.id
ORDER BY u.id;

-- LEFT JOIN
-- name: GetUserOrders :many
SELECT
    u.id,
    u.name,
    o.id as order_id,
    o.amount
FROM users u
LEFT JOIN orders o ON u.id = o.user_id
WHERE u.id = ?
ORDER BY o.created_at DESC;

-- Multiple JOINs
-- name: GetCompleteOrderInfo :one
SELECT
    o.id,
    o.amount,
    u.id as user_id,
    u.name as user_name,
    d.id as department_id,
    d.name as department_name
FROM orders o
INNER JOIN users u ON o.user_id = u.id
INNER JOIN departments d ON u.department_id = d.id
WHERE o.id = ?;

-- RIGHT JOIN
-- name: GetDepartmentsWithUsers :many
SELECT
    d.id,
    d.name,
    COUNT(u.id) as user_count
FROM departments d
RIGHT JOIN users u ON d.id = u.department_id
GROUP BY d.id, d.name
ORDER BY user_count DESC;

-- Multiple CTEs with unqualified GROUP BY columns
-- name: GetResourceStatus :many
WITH current_status AS (
    SELECT department_id, COUNT(users.id) as user_count, MAX(users.id) as latest_id
    FROM users
    WHERE users.id > ?
    GROUP BY department_id
),
department_details AS (
    SELECT departments.id, departments.name
    FROM departments
    WHERE departments.id IN (SELECT department_id FROM users WHERE users.id > ?)
)
SELECT
    cs.department_id,
    cs.user_count,
    cs.latest_id,
    dd.name as department_name
FROM current_status cs
LEFT JOIN department_details dd ON cs.department_id = dd.id
ORDER BY cs.user_count DESC;
