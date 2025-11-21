-- ClickHouse example queries
-- ClickHouse supports both positional (?) and named (sqlc.arg / sqlc.narg / sqlc.slice) parameters

-- Positional parameter examples
-- name: GetUserByID :one
SELECT id, name, email, created_at
FROM sqlc_example.users
WHERE id = ?;

-- name: ListUsers :many
SELECT id, name, email, created_at
FROM sqlc_example.users
ORDER BY created_at DESC
LIMIT ?;

-- name: InsertUser :exec
INSERT INTO sqlc_example.users (id, name, email, created_at)
VALUES (?, ?, ?, ?);

-- Named parameter examples using sqlc.arg() function
-- name: GetUserByEmail :one
SELECT id, name, email, created_at
FROM sqlc_example.users
WHERE email = sqlc.arg('email');

-- name: InsertUserNamed :exec
INSERT INTO sqlc_example.users (id, name, email, created_at)
VALUES (sqlc.arg('id'), sqlc.arg('name'), sqlc.arg('email'), sqlc.arg('created_at'));

-- name: GetUserPostsForUser :many
SELECT p.id, p.user_id, p.title, p.content, p.created_at
FROM sqlc_example.posts p
WHERE p.user_id = sqlc.arg('user_id')
ORDER BY p.created_at DESC;

-- name: InsertPost :exec
INSERT INTO sqlc_example.posts (id, user_id, title, content, created_at)
VALUES (sqlc.arg('id'), sqlc.arg('user_id'), sqlc.arg('title'), sqlc.arg('content'), sqlc.arg('created_at'));

-- name: GetCommentsForPost :many
SELECT id, post_id, user_id, content, created_at
FROM sqlc_example.comments
WHERE post_id = sqlc.arg('post_id')
ORDER BY created_at ASC;

-- name: InsertComment :exec
INSERT INTO sqlc_example.comments (id, post_id, user_id, content, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetUserWithPosts :many
SELECT u.id, u.name, u.email, u.created_at, p.id as post_id, p.title
FROM sqlc_example.users u
LEFT JOIN sqlc_example.posts p ON u.id = p.user_id
WHERE u.id = sqlc.arg('user_id')
ORDER BY p.created_at DESC;

-- Named parameter with nullable values using sqlc.narg()
-- name: GetPostsByOptionalStatus :many
SELECT id, user_id, title, status, created_at
FROM sqlc_example.posts
WHERE (sqlc.narg('status') IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC;

-- ClickHouse-specific aggregate functions

-- name: GetUserAnalytics :many
SELECT 
	u.id,
	u.name,
	COUNT(*) as total_posts,
	uniqExact(p.id) as unique_posts,
	countIf(p.created_at >= toDate(now()) - 30) as posts_last_30_days,
	argMax(p.title, p.created_at) as latest_post_title,
	argMaxIf(p.title, p.created_at, p.created_at >= toDate(now()) - 30) as latest_post_in_30_days
FROM sqlc_example.users u
LEFT JOIN sqlc_example.posts p ON u.id = p.user_id
GROUP BY u.id, u.name
HAVING COUNT(*) > 0
ORDER BY total_posts DESC;

-- name: GetCommentAnalytics :many
SELECT 
	p.id as post_id,
	p.title,
	COUNT(*) as total_comments,
	uniqExact(c.user_id) as unique_commenters,
	countIf(c.created_at >= toDate(now()) - 7) as comments_last_week,
	argMin(c.created_at, c.id) as first_comment_time,
	argMax(c.created_at, c.id) as last_comment_time
FROM sqlc_example.posts p
LEFT JOIN sqlc_example.comments c ON p.id = c.post_id
WHERE p.user_id = sqlc.arg('user_id')
GROUP BY p.id, p.title
ORDER BY total_comments DESC;

-- Statistical aggregate functions

-- name: GetMetricsStatistics :many
SELECT 
	category,
	COUNT(*) as count,
	varSamp(value) as variance_sample,
	varPop(value) as variance_population,
	stddevSamp(value) as stddev_sample,
	stddevPop(value) as stddev_population,
	corr(value_x, value_y) as correlation
FROM sqlc_example.metrics
WHERE timestamp >= sqlc.arg('start_time') AND timestamp <= sqlc.arg('end_time')
GROUP BY category
ORDER BY count DESC;

-- Conditional aggregate variants

-- name: GetOrderMetrics :many
SELECT 
	status,
	COUNT(*) as total_orders,
	minIf(amount, amount > 0) as min_positive_amount,
	maxIf(amount, amount > 0) as max_positive_amount,
	sumIf(amount, status = 'completed') as completed_revenue,
	avgIf(rating, rating IS NOT NULL) as avg_valid_rating
FROM sqlc_example.orders
WHERE created_at >= sqlc.arg('start_date')
GROUP BY status
ORDER BY total_orders DESC;

-- IN operator with multiple conditions

-- name: FilterUsersByIDAndStatus :many
SELECT id, name, email, status, created_at
FROM sqlc_example.users
WHERE id IN (sqlc.slice('user_ids'))
AND status IN ('active', 'pending')
ORDER BY created_at DESC;

-- ORDER BY with WITH FILL for time series

-- name: GetTimeSeriesWithFill :many
SELECT date, metric_value
FROM sqlc_example.timeseries
WHERE date >= sqlc.arg('start_date') AND date <= sqlc.arg('end_date')
ORDER BY date WITH FILL FROM sqlc.arg('start_date') TO sqlc.arg('end_date');

-- Type casting examples

-- name: GetCastedValues :many
SELECT 
	id::String as id_text,
	amount::Float32 as amount_float,
	created_at::Date as date_only,
	status::String as status_text
FROM sqlc_example.events
WHERE created_at::Date >= sqlc.arg('date_filter');

-- ARRAY JOIN examples

-- name: UnfoldUserTags :many
SELECT 
	u.id as user_id,
	u.name as user_name,
	tag
FROM sqlc_example.users_with_tags u
ARRAY JOIN u.tags AS tag
WHERE u.id = sqlc.arg('user_id')
ORDER BY tag;

-- name: UnfoldEventProperties :many
SELECT 
	e.event_id,
	e.event_name,
	e.timestamp,
	prop_key,
	prop_value
FROM sqlc_example.events_with_properties e
ARRAY JOIN e.properties.keys AS prop_key, e.properties.values AS prop_value
WHERE e.timestamp >= sqlc.arg('start_time')
ORDER BY e.timestamp DESC;

-- name: UnfoldNestedData :many
SELECT 
	record_id,
	nested_value
FROM sqlc_example.nested_table
ARRAY JOIN nested_array AS nested_value
WHERE record_id IN (sqlc.slice('record_ids'));

-- name: AnalyzeArrayElements :many
SELECT 
	product_id,
	arrayJoin(categories) AS category,
	COUNT(*) OVER (PARTITION BY category) as category_count
FROM sqlc_example.products
WHERE product_id = ?
GROUP BY product_id, category;

-- name: ExtractMetadataFromJSON :many
SELECT 
	MetadataPlatformId,
	arrayJoin(JSONExtract(JsonValue, 'Array(String)')) as self_help_id
FROM sqlc_example.events;


