# sqlc.slice() Bug in ClickHouse Code Generation

## The Problem

Generated Go code contains invalid ClickHouse SQL:
```go
const filterUsersByIDAndStatus = `
WHERE id IN (sqlc.slice('user_ids'))  // ❌ NOT valid ClickHouse SQL!
`
```

This would **fail at runtime** because ClickHouse doesn't understand `sqlc.slice()`.

## Root Cause

The issue is caused by the **order of operations** in the ClickHouse parser:

### Current Flow (BROKEN)

1. **Parser preprocessing** (internal/engine/clickhouse/parse.go:56)
   ```go
   processedSQL := preprocessNamedParameters(originalSQL)
   // "sqlc.slice('user_ids')" → "?"
   ```

2. **Parse preprocessed SQL** → Creates AST with `ParamRef` nodes
   - AST contains: `WHERE id IN (?)`
   - Original function call information is LOST

3. **Compiler runs rewrite.NamedParameters** (internal/compiler/analyze.go:134)
   - Looks for `sqlc.slice()` FUNCTION CALLS in AST
   - Finds NONE (they were removed by preprocessing!)
   - Creates ZERO edits

4. **source.Mutate applies edits** (internal/compiler/analyze.go:190)
   - No edits to apply
   - Returns ORIGINAL query string unchanged

5. **Generated code embeds original query**
   ```go
   const query = `WHERE id IN (sqlc.slice('user_ids'))` // ❌ BUG!
   ```

### Why Other Engines Don't Have This Problem

**PostgreSQL:**
- Parser natively supports the syntax
- No preprocessing needed
- `rewrite.NamedParameters` finds function calls in AST
- Creates edits: `sqlc.slice('ids')` → `$1`

**MySQL:**
- Parser natively supports the syntax (uses TiDB parser)
- No preprocessing needed
- `rewrite.NamedParameters` finds function calls in AST
- Creates edits: `sqlc.slice('ids')` → `/*SLICE:ids*/?`

**ClickHouse:**
- Parser library doesn't support the syntax
- Preprocessing is required to parse
- But preprocessing DESTROYS the information needed for edits!

## Why Tests Don't Catch This

1. **Parser tests** only verify parsing succeeds - they don't execute queries
2. **Code generation tests** only verify Go code compiles - they don't run it
3. **No runtime integration tests** that execute the generated code against ClickHouse
4. **Integration tests exist** but require `-tags=integration` and a running database

## Evidence

```bash
$ go run test_sqlc_slice.go
Original query:
SELECT id FROM users WHERE id IN (sqlc.slice('user_ids'))

Extracted SQL from parser:
SELECT id FROM users WHERE id IN (sqlc.slice('user_ids'))

Dollar: true, Numbers: map[]

Edits from NamedParameters: 0

⚠️  No edits created! The query will be embedded as-is with sqlc.slice()
```

## The Fix

We need to create the edits DURING preprocessing, not after. The preprocessor needs to:

1. Track the original locations of `sqlc.slice()` calls
2. Create `source.Edit` objects to replace them
3. Return both the preprocessed SQL AND the edits
4. Apply the edits to the ORIGINAL SQL for embedding in generated code

### Proposed Solution

**Option 1: Return Edits from Preprocessing**

Modify `internal/engine/clickhouse/parse.go`:

```go
func preprocessNamedParameters(sql string) (string, []source.Edit) {
    var edits []source.Edit

    // Track sqlc.slice positions BEFORE replacing
    funcPattern := regexp.MustCompile(`sqlc\.(arg|narg|slice)\s*\(\s*['"]([^'"]+)['"]\s*\)`)
    matches := funcPattern.FindAllStringSubmatchIndex(sql, -1)

    for _, match := range matches {
        funcType := sql[match[2]:match[3]]  // arg, narg, or slice
        paramName := sql[match[4]:match[5]]  // parameter name

        var replacement string
        if funcType == "slice" {
            replacement = fmt.Sprintf("/*SLICE:%s*/?", paramName)
        } else {
            replacement = "?"
        }

        edits = append(edits, source.Edit{
            Location: match[0],
            Old:      sql[match[0]:match[1]],
            New:      replacement,
        })
    }

    // Also handle @param syntax
    // ...

    // Apply replacements for parsing
    processedSQL := funcPattern.ReplaceAllString(sql, "?")
    // ...

    return processedSQL, edits
}
```

Then in `Parse()`:
```go
processedSQL, edits := preprocessNamedParameters(originalSQL)

// Store edits for later use by compiler
// Attach to RawStmt somehow, or return separately
```

**Option 2: Embed Preprocessed SQL Instead of Original**

Simpler but less ideal - embed the preprocessed SQL (with `?`) instead of original:

```go
// In parse.go, use processedSQL for the segment:
segment := processedSQL[statementStart:statementEnd]
```

But this loses the original query which might be useful for debugging.

**Option 3: Don't Preprocess in Parser**

Move preprocessing to a different layer, but this requires the ClickHouse parser library to support the syntax (which it doesn't).

## Recommended Fix

**Option 1** - Return edits from preprocessing and apply them to the original SQL before embedding.

This matches how other engines work:
1. Parse creates AST
2. Compiler creates edits based on AST
3. Edits are applied to original SQL
4. Modified SQL is embedded in generated code

For ClickHouse:
1. Preprocessing creates edits based on string matching
2. Parse creates AST from preprocessed SQL
3. Compiler may create additional edits
4. ALL edits are applied to original SQL
5. Modified SQL is embedded in generated code

## Impact

**Severity:** High
**Affected:** All ClickHouse queries using `sqlc.slice()`, `sqlc.arg()`, or `@param` syntax
**Runtime Behavior:** Query execution would fail with SQL syntax error

## Test to Add

```go
func TestSqlcSliceCodeGeneration(t *testing.T) {
    // Parse query with sqlc.slice
    // Generate code
    // Verify embedded query has /*SLICE:param*/? not sqlc.slice()
}
```

Or better: add runtime integration test that executes generated code.
