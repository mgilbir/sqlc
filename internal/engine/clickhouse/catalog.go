package clickhouse

import (
	"github.com/sqlc-dev/sqlc/internal/sql/catalog"
)

// NewCatalog creates a new ClickHouse catalog with default settings
func NewCatalog() *catalog.Catalog {
	// ClickHouse uses "default" as the default database
	defaultSchemaName := "default"

	return &catalog.Catalog{
		DefaultSchema: defaultSchemaName,
		Schemas: []*catalog.Schema{
			newDefaultSchema(defaultSchemaName),
		},
		Extensions: map[string]struct{}{},
	}
}

// newDefaultSchema creates the default ClickHouse schema
func newDefaultSchema(name string) *catalog.Schema {
	return &catalog.Schema{
		Name:   name,
		Tables: make([]*catalog.Table, 0),
	}
}
