-- ClickHouse example schema

CREATE DATABASE IF NOT EXISTS sqlc_example;

CREATE TABLE IF NOT EXISTS sqlc_example.users
(
    id UInt32,
    name String,
    email String,
    created_at DateTime
)
ENGINE = MergeTree()
ORDER BY id;

CREATE TABLE IF NOT EXISTS sqlc_example.posts
(
    id UInt32,
    user_id UInt32,
    title String,
    content String,
    created_at DateTime
)
ENGINE = MergeTree()
ORDER BY (id, user_id);

CREATE TABLE IF NOT EXISTS sqlc_example.comments
(
    id UInt32,
    post_id UInt32,
    user_id UInt32,
    content String,
    created_at DateTime
)
ENGINE = MergeTree()
ORDER BY (id, post_id, user_id);

-- Tables with array columns for ARRAY JOIN examples

CREATE TABLE IF NOT EXISTS sqlc_example.users_with_tags
(
    id UInt32,
    name String,
    tags Array(String)
)
ENGINE = MergeTree()
ORDER BY id;

CREATE TABLE IF NOT EXISTS sqlc_example.events_with_properties
(
    event_id UInt32,
    event_name String,
    timestamp DateTime,
    properties Nested(
        keys String,
        values String
    )
)
ENGINE = MergeTree()
ORDER BY event_id;

CREATE TABLE IF NOT EXISTS sqlc_example.nested_table
(
    record_id UInt32,
    nested_array Array(String)
)
ENGINE = MergeTree()
ORDER BY record_id;

CREATE TABLE IF NOT EXISTS sqlc_example.products
(
    product_id UInt32,
    name String,
    categories Array(String)
)
ENGINE = MergeTree()
ORDER BY product_id;

CREATE TABLE IF NOT EXISTS sqlc_example.metrics
(
    category String,
    value Float64,
    value_x Float64,
    value_y Float64,
    timestamp DateTime
)
ENGINE = MergeTree()
ORDER BY timestamp;

CREATE TABLE IF NOT EXISTS sqlc_example.orders
(
    status String,
    amount Float64,
    rating Nullable(Float64),
    created_at DateTime
)
ENGINE = MergeTree()
ORDER BY created_at;

CREATE TABLE IF NOT EXISTS sqlc_example.timeseries
(
    date Date,
    metric_value Float64
)
ENGINE = MergeTree()
ORDER BY date;

CREATE TABLE IF NOT EXISTS sqlc_example.events
(
    id UInt32,
    amount Float64,
    created_at DateTime,
    status String
)
ENGINE = MergeTree()
ORDER BY id;
