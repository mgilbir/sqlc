# Plan: Remove @param Support and Refactor sqlc.* Functions for ClickHouse

## Overview

This plan addresses two interconnected issues with ClickHouse named parameter handling:

1. **Remove `@param` syntax** - Drop support for `@param_name` style parameters
2. **Convert `sqlc.*` functions to `sqlc_*`** - Transform `sqlc.arg()`, `sqlc.narg()`, `sqlc.slice()` to `sqlc_arg()`, `sqlc_narg()`, `sqlc_slice()` so the ClickHouse parser can recognize them as function calls instead of failing on the dot notation

## Current Scope of sqlc.* Functions

The codebase currently supports three named parameter functions:
- **`sqlc.arg('name')`** - Required parameter
- **`sqlc.narg('name')`** - Optional (nullable) parameter  
- **`sqlc.slice('name')`** - Array/slice parameter for IN clauses

These are used across **all database engines**: PostgreSQL, MySQL, SQLite, and ClickHouse.

## Detailed Implementation Plan

### Phase 1: ClickHouse Parser Preprocessing

**File:** `internal/engine/clickhouse/parse.go`

#### 1.1 Remove @param Preprocessing
- Remove the `@[a-zA-Z_][a-zA-Z0-9_]*` regex pattern
- Remove handling of `@identifier` in `findOriginalPosition()`

#### 1.2 Add sqlc.* to sqlc_* Conversion
- Convert `sqlc.arg(...)` → `sqlc_arg(...)`
- Convert `sqlc.narg(...)` → `sqlc_narg(...)`
- Convert `sqlc.slice(...)` → `sqlc_slice(...)`
- Use regex replacement since the function names have the same length (9 chars: "sqlc." = "sqlc_")

**Implementation approach:**
```go
// In preprocessNamedParameters():
// Old: sqlc.arg(...) → ?
// New: sqlc.arg(...) → sqlc_arg(...) → FunctionExpr in AST → ConvertFunctionExpr handles it

// Replace sqlc.arg(...) with sqlc_arg(...) 
funcPattern := regexp.MustCompile(`sqlc\.(arg|narg|slice)\s*\(`)
sql = funcPattern.ReplaceAllString(sql, "sqlc_$1(")
```

**Benefits of same-length replacement:**
- Text positions remain stable
- `findOriginalPosition()` position mapping logic is simplified (no need to track length differences)
- String replacement in subsequent stages (star expansion, parameter rewriting) work without offset adjustments

#### 1.3 Simplify Position Mapping
- Remove `sqlc.arg` pattern handling in `findOriginalPosition()` since we no longer preprocess function calls into `?`
- Remove complex logic tracking preprocessing-induced position differences
- The position mapping becomes straightforward character-by-character comparison (no @param or sqlc.* preprocessing changes lengths)

### Phase 2: ClickHouse AST Converter Enhancement

**File:** `internal/engine/clickhouse/convert.go`

#### 2.1 Handle `sqlc_*` Function Calls
In `convertFunctionExpr()`, recognize and specially handle `sqlc_arg`, `sqlc_narg`, and `sqlc_slice`:

```go
func (c *cc) convertFunctionExpr(fn *chparser.FunctionExpr) ast.Node {
    funcName := identifier(fn.Name.Name)
    
    // Detect and normalize sqlc_* functions back to sqlc.* for sqlc AST
    if strings.HasPrefix(funcName, "sqlc_") {
        funcName = "sqlc." + strings.TrimPrefix(funcName, "sqlc_")
    }
    
    // ... rest of conversion
    args := &ast.List{Items: []ast.Node{}}
    if fn.Params != nil {
        if fn.Params.Items != nil {
            for _, item := range fn.Params.Items.Items {
                args.Items = append(args.Items, c.convert(item))
            }
        }
    }
    
    return &ast.FuncCall{
        Func: &ast.FuncName{
            Schema: "sqlc",
            Name: strings.TrimPrefix(funcName, "sqlc."),  // "arg", "narg", or "slice"
        },
        Funcname: &ast.List{
            Items: []ast.Node{
                &ast.String{Str: funcName},  // "sqlc.arg", "sqlc.narg", or "sqlc.slice"
            },
        },
        Args: args,
    }
}
```

### Phase 3: Validation & Rewriting (No Changes Needed)

**Files affected:** `internal/sql/named/*`, `internal/sql/rewrite/*`, `internal/sql/validate/*`

These components work with the sqlc AST (post-conversion), so they'll automatically handle the converted functions correctly:

- `internal/sql/named/is.go:IsParamFunc()` - Checks for `call.Func.Schema == "sqlc"` and names in `["arg", "narg", "slice"]`
- `internal/sql/rewrite/parameters.go:NamedParameters()` - Converts sqlc functions to database-specific placeholders
- `internal/sql/validate/*` - Validates parameter usage

**No code changes needed** because:
1. ClickHouse converter will output proper `ast.FuncCall` nodes with `Schema: "sqlc"` and function names
2. The rest of the pipeline validates/rewrites based on the sqlc AST, not raw SQL

### Phase 4: Testing

#### 4.1 Unit Tests to Update
- `internal/engine/clickhouse/parse_test.go`
  - Remove `TestParseNamedParameterAtSign` test
  - Update `TestParseNamedParameterSqlcArg`, `TestParseNamedParameterSqlcNarg`, `TestParseNamedParameterSqlcSlice`
  - Update `TestParseNamedParameterMixed` (remove @param examples)
  - These should now verify that:
    - Original SQL has `sqlc.arg(...)` format
    - Parser preprocesses to `sqlc_arg(...)`
    - AST conversion normalizes back to `sqlc.arg(...)` in function schema/name

#### 4.2 Integration Tests
- `internal/engine/clickhouse/parse_actual_queries_test.go`
  - Already contains `sqlc.slice('record_ids')` test, should continue to work

#### 4.3 Example Tests  
- `examples/clickhouse/queries.sql`
  - Update examples to use only `sqlc.arg()`, `sqlc.narg()`, `sqlc.slice()`
  - Remove any `@param` examples

## Implementation Order

1. **Step 1:** Update `internal/engine/clickhouse/parse.go`
   - Remove @param preprocessing
   - Add sqlc.* to sqlc_* conversion
   - Simplify `findOriginalPosition()` and `findOriginalSQL()`

2. **Step 2:** Update `internal/engine/clickhouse/convert.go`
   - Enhance `convertFunctionExpr()` to normalize sqlc_* functions

3. **Step 3:** Update tests
   - Remove @param test cases
   - Update existing test assertions
   - Verify position calculations work correctly

4. **Step 4:** Test end-to-end
   - Run `make test` for ClickHouse tests
   - Run `make test-ci` for full suite
   - Verify star expansion works (position mapping)
   - Verify parameter detection works in rewrite phase

## Code Changes Summary

### Files to Modify

1. **`internal/engine/clickhouse/parse.go`**
   - Remove @param handling (~10 lines)
   - Modify `preprocessNamedParameters()` to use sqlc.* to sqlc_* conversion
   - Simplify `findOriginalPosition()` (~15 lines of logic removed)

2. **`internal/engine/clickhouse/convert.go`**
   - Enhance `convertFunctionExpr()` with sqlc_* normalization (~5 lines)

3. **`internal/engine/clickhouse/parse_test.go`**
   - Update test expectations (~20 lines)
   - Remove @param tests

### Files NOT Modified

- `internal/sql/named/*` - Works with sqlc AST
- `internal/sql/rewrite/*` - Works with sqlc AST
- `internal/sql/validate/*` - Works with sqlc AST
- `internal/compiler/*` - Works with sqlc AST
- `internal/codegen/*` - Works with sqlc AST

## Risk Assessment

**Low Risk** because:
1. Changes are localized to ClickHouse parser preprocessing and conversion
2. Rest of pipeline unchanged
3. Same-length function name replacement preserves string positions
4. All other database engines unaffected

**Test Coverage:**
- Unit tests for parsing verify AST structure
- Integration tests verify end-to-end compilation
- Example tests verify code generation

## Success Criteria

1. ✅ @param syntax no longer parsed (rejection or conversion fails appropriately)
2. ✅ `sqlc.arg()`, `sqlc.narg()`, `sqlc.slice()` parse correctly
3. ✅ Parser preprocesses them to `sqlc_arg()`, `sqlc_narg()`, `sqlc_slice()`
4. ✅ Converter produces AST nodes with `Schema: "sqlc"` and proper function names
5. ✅ All downstream validation/rewriting works
6. ✅ Star expansion still works (position mapping correct)
7. ✅ Parameter inference still works
8. ✅ All tests pass (`make test-ci`)
