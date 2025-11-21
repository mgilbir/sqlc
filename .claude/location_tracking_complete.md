# ClickHouse AST Location Tracking - Complete

## Status: ✅ COMPLETE

All critical AST node types in the ClickHouse engine now properly track location information from the ClickHouse parser.

## What Was Fixed

### New Location Tracking Added
1. **ColumnRef** (nested identifiers like `database.table`)
   - `convertNestedIdentifier()` now sets `Location: int(nested.Pos())`
   - Affects: table references with schema qualification

2. **SortBy** (ORDER BY expressions)
   - `convertOrderExpr()` now sets `Location: int(order.Pos())`
   - Affects: ordering clauses in SELECT queries

3. **CommonTableExpr** (WITH clause CTEs)
   - `convertCTE()` now sets `Location: int(cte.Pos())`
   - Affects: common table expressions in WITH clauses

4. **MapLiteral A_Const** (ClickHouse map literals)
   - `convertMapLiteral()` now sets `Location: int(mapLit.Pos())`
   - Affects: ClickHouse-specific map/dictionary syntax

### Already Had Location Tracking (Previous Fixes)
- FuncCall - function calls
- A_Expr - binary and unary operations
- A_Const - number and string literals
- WindowDef - window function definitions
- NullTest - IS NULL/IS NOT NULL expressions
- CaseExpr - CASE expressions
- TypeCast - CAST expressions
- ParamRef - parameter references (both `?` and named)
- ResTarget - SELECT target list items
- RangeVar - table/schema references
- WithClause - WITH clause container

## Why This Matters

Location tracking is critical for:

1. **Star Expansion** - The compiler needs to know where `*` appears in queries to expand it correctly
2. **Named Parameter Detection** - Position tracking helps identify `sqlc.arg()`, `sqlc.narg()`, and `sqlc.slice()` calls
3. **Error Reporting** - Precise positions enable meaningful error messages with line/column information
4. **Query Processing** - Downstream transformations rely on accurate position information

## Implementation Pattern

All fixes follow the same consistent pattern:

```go
return &ast.NodeType{
    Field1: value,
    Field2: value,
    Location: int(clickhouseNode.Pos()),  // ← Set Location from ClickHouse parser
}
```

This ensures positions flow from the ClickHouse parser → sqlc AST → downstream processing.

## Test Coverage

- ✅ 59 ClickHouse unit tests pass
- ✅ Compiler integration tests pass
- ✅ Complex queries with CTEs, JOINs, window functions work correctly
- ✅ Named parameter detection works properly
- ✅ Array join and aggregate functions tested

## Files Modified

- `internal/engine/clickhouse/convert.go` - Added Location to 4 node types

## No Breaking Changes

All changes are additive - they only add Location information to nodes that already existed. No AST structure changes, no API changes.
