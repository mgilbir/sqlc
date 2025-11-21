# SQL Query Processing Flow in sqlc

This document maps how a query flows from input SQL files through parsing, AST conversion, analysis, and finally to generated code.

## High-Level Overview

```
Input SQL Files
    ↓
Parser (ClickHouse, PostgreSQL, MySQL, SQLite)
    ↓
Database-Specific AST
    ↓
Convert to sqlc Internal AST
    ↓
Query Analysis & Inference
    ↓
Text Manipulation (Star Expansion, Parameter Rewriting)
    ↓
Generated Code Output
```

---

## Detailed Stage-by-Stage Flow

### Stage 1: File Reading & Compiler Setup

**Location:** `internal/cmd/generate.go`

```
generate.go:Generate()
    ├─ readConfig()  → Reads sqlc.yaml/sqlc.json
    └─ parse()       → Creates Compiler instance for each engine
```

**For ClickHouse Specifically:**

`internal/compiler/engine.go:NewCompiler()`
```go
case config.EngineClickHouse:
    c.parser = clickhouse.NewParser()      // Create CH parser
    c.catalog = clickhouse.NewCatalog()    // Create CH catalog
    c.selector = newDefaultSelector()      // Create query selector
```

---

### Stage 2: Schema Parsing (Catalog Initialization)

**Location:** `internal/compiler/compile.go`

```
ParseCatalog()
    ↓
parseCatalog()
    ↓
For each schema file:
    ├─ c.parser.Parse(file)  → Calls appropriate parser
    └─ Update c.catalog with schema information
```

**For ClickHouse:**

`internal/engine/clickhouse/parse.go:Parse()`

1. **Read input** → Read all bytes from schema.sql file
2. **Preprocess named parameters** (if any) → Convert `@param` to `?`
3. **Create ClickHouse parser** → `chparser.NewParser(sql)`
4. **Parse statements** → `chp.ParseStmts()` returns ClickHouse AST nodes

```
ClickHouse SQL (schema.sql)
    ↓
preprocessNamedParameters()
    [Converts @name, sqlc.arg(), sqlc.narg(), sqlc.slice() → ?]
    ↓
chparser.NewParser() + ParseStmts()
    [Produces: chparser.CreateTable, chparser.AlterTable, etc.]
    ↓
Convert to sqlc AST
    [Via converter.convert() for each statement]
```

---

### Stage 3: ClickHouse AST → sqlc Internal AST Conversion

**Location:** `internal/engine/clickhouse/convert.go`

**The Converter Pattern:**

```go
type cc struct {
    paramCount int
}

func (c *cc) convert(node chparser.Expr) ast.Node {
    switch n := node.(type) {
    case *chparser.SelectQuery:
        return c.convertSelectQuery(n)
    case *chparser.InsertStmt:
        return c.convertInsertStmt(n)
    case *chparser.CreateTable:
        return c.convertCreateTable(n)
    // ... more statement types
    
    case *chparser.BinaryOperation:
        return c.convertBinaryOperation(n)
    case *chparser.FunctionExpr:
        return c.convertFunctionExpr(n)
    // ... more expression types
    }
}
```

**Key Conversion Points:**

| ClickHouse Type | sqlc Internal Type | Example |
|---|---|---|
| `chparser.SelectQuery` | `ast.SelectStmt` | `SELECT id, name FROM users` |
| `chparser.InsertStmt` | `ast.InsertStmt` | `INSERT INTO users VALUES (...)` |
| `chparser.FunctionExpr` | `ast.FuncCall` | `COUNT(*)`, `SUM(amount)` |
| `chparser.BinaryOperation` | `ast.A_Expr` | `WHERE id > 10` |
| `chparser.PlaceHolder` | `ast.ParamRef` | `?` (after preprocessing @name) |

**Location tracking in convert.go:**

The parser tracks statement positions in the original SQL:
```go
// In parse.go:
for i := range stmtNodes {
    converter := &cc{}
    out := converter.convert(stmtNodes[i])
    
    // Calculate statement location in original SQL
    statementStart = nameCommentPositions[i]
    statementEnd = nameCommentPositions[i+1]
    
    // Create RawStmt with location info
    stmts = append(stmts, &ast.RawStmt{
        Stmt:        out,
        StmtLocation: statementStart,
        StmtLen:      statementEnd - statementStart,
    })
}
```

---

### Stage 4: Query Parsing & Metadata Extraction

**Location:** `internal/compiler/parse.go:parseQuery()`

```
parseQuery(stmt ast.Node, src string)
    ↓
Extract query name and type from SQL comments:
    validate.SqlcFunctions()
    ↓
    source.Pluck()  → Extract raw SQL between statement positions
    ↓
    metadata.ParseQueryNameAndType()  → Extract -- name: <query_name> <cmd>
    ↓
    metadata.ParseCommentFlags()      → Parse special sqlc directives
```

**Example:**
```sql
-- name: GetUsers :many
SELECT id, name FROM users WHERE status = @status;
```

Becomes metadata:
```
name: "GetUsers"
cmd: ":many"
params: [...]
flags: [...]
```

---

### Stage 5: Query Inference (Initial Analysis)

**Location:** `internal/compiler/analyze.go:inferQuery()`

```
inferQuery(raw *ast.RawStmt, query string)
    ↓
_analyzeQuery(raw, query, failfast=false)  ← Non-strict mode
    ↓
validate.ParamRef()          → Check parameter references are valid
    ↓
rewrite.NamedParameters()    → Convert @param to $1, $2 style
    [Returns: AST with positional params, namedParams set]
    ↓
For SELECT:
    └─ Find table references from AST
    └─ Determine output columns
```

**Key rewrite: Named Parameters**

`internal/sql/rewrite/named_parameters.go`

Transforms:
```sql
SELECT * FROM users WHERE id = @user_id
```

To internal AST representation with:
- Named parameter info preserved for code generation
- AST with positional placeholders for validation

---

### Stage 6: Expand Stars (Text Manipulation #1)

**Location:** `internal/compiler/expand.go:expand()`

```
expand(qc *QueryCatalog, raw *ast.RawStmt)
    ↓
Search AST for A_Star nodes (SELECT *)
    ↓
For each SELECT *:
    ├─ Find source tables from query
    ├─ Look up table columns in catalog
    ├─ Generate column list from catalog schema
    └─ Create source.Edit to replace * with explicit columns
```

**Example:**

Input:
```sql
SELECT * FROM users;
```

Output (after star expansion):
```
SELECT id, name, email FROM users;
```

Generated `source.Edit`:
```
Position: Location of '*'
Text: "id, name, email"
```

These edits are accumulated and applied to the original SQL text.

---

### Stage 7: Full Query Analysis (Engine-Specific)

**Location:** `internal/compiler/analyze.go:analyzeQuery()`

```
analyzeQuery(raw *ast.RawStmt, query string)
    ↓
_analyzeQuery(raw, query, failfast=true)  ← Strict mode
    ↓
rewrite.NamedParameters()
    ↓
Apply star expansion edits to query string
    [If SELECT *, apply generated edits]
    ↓
c.analyzer.Analyze()  ← Engine-specific analyzer
    [PostgreSQL: Uses PostgreSQL analyzer]
    [MySQL: Uses TiDB-based analyzer]
    [SQLite: Uses sqlite parser analyzer]
    [ClickHouse: Uses basic analyzer]
    ↓
Returns: analyzer.Analysis
    └─ Columns with types
    └─ Parameters with types
    └─ Table references
```

---

### Stage 8: Text Manipulation #2 - Identifier Quoting

**Location:** `internal/compiler/expand.go:quote()`

When expanding columns, identifiers are quoted based on database:

```go
func (c *Compiler) quote(x string) string {
    switch c.conf.Engine {
    case config.EngineClickHouse:
        return "`" + x + "`"      // ClickHouse uses backticks
    case config.EngineMySQL:
        return "`" + x + "`"      // MySQL uses backticks
    default:
        return "\"" + x + "\""    // PostgreSQL uses double quotes
    }
}
```

**Why this matters for ClickHouse:**

- ClickHouse is case-sensitive for identifiers
- Backtick quoting is required for reserved words or special names
- When expanding `SELECT *`, generated columns must be backtick-quoted

---

### Stage 9: Final Query Compilation

**Location:** `internal/compiler/parse.go:parseQuery()` (continued)

```
After analysis:

Combine analysis results:
    ├─ Column information from analyzer
    ├─ Parameter information from analyzer
    ├─ Named parameters mapping
    └─ Final query text (with expansions applied)

Create Query object:
    ├─ Name: "GetUsers"
    ├─ Text: Final SQL with all rewrites
    ├─ Columns: Output columns with types
    ├─ Params: Input parameters with types
    ├─ Cmd: ":many", ":one", ":exec", ":execrows"
    └─ Comments: Metadata from headers
```

---

### Stage 10: Code Generation

**Location:** `internal/cmd/generate.go:codegen()`

```
codegen(ctx context.Context, result *compiler.Result)
    ↓
Determine code generator:
    ├─ Go codegen (golang/)
    ├─ JSON codegen
    └─ Plugin system (gRPC)
    ↓
For Go code generation:
    [internal/codegen/golang/gen.go]
    ├─ Generate db.go
    │   └─ Database connection/transaction handling
    ├─ Generate models.go
    │   └─ Type definitions for output columns
    ├─ Generate <query_name>.sql.go
    │   ├─ Query interface
    │   ├─ Query runner function
    │   ├─ Argument struct (for named params)
    │   └─ Result struct (for output columns)
    └─ Generate sqlc.yml schema mapping
```

**Type Mapping to Go:**

`internal/codegen/golang/go_type.go`

```go
func goInnerType(req *plugin.GenerateRequest, options *opts.Options, col *plugin.Column) string {
    // Check overrides first
    // Then switch on engine:
    switch req.Settings.Engine {
    case "mysql":
        return mysqlType(req, options, col)
    case "postgresql":
        return postgresType(req, options, col)
    case "sqlite":
        return sqliteType(req, options, col)
    case "clickhouse":
        return clickhouseType(req, options, col)  // ← NEW
    default:
        return "interface{}"
    }
}
```

**For ClickHouse:**

`internal/codegen/golang/clickhouse_type.go`

Maps ClickHouse types to Go types with nullability handling:

```
ClickHouse Type          → Go Type (nullable)      → Go Type (not null)
─────────────────────────────────────────────────────────────────
String, VARCHAR          → sql.NullString          → string
UInt8, UInt16, UInt32    → sql.NullInt32/64        → uint32
Int8, Int16, Int32       → sql.NullInt16/32/64     → int32
Float32, Float64         → sql.NullFloat64         → float64
Date, DateTime64         → sql.NullTime            → time.Time
Boolean                  → sql.NullBool            → bool
```

Driver selection:

```go
func parseDriver(sqlPackage string) opts.SQLDriver {
    switch sqlPackage {
    case opts.SQLPackagePGXV4:
        return opts.SQLDriverPGXV4
    case opts.SQLPackagePGXV5:
        return opts.SQLDriverPGXV5
    case opts.SQLPackageClickHouseV2:         // ← NEW
        return opts.SQLDriverClickHouseV2
    default:
        return opts.SQLDriverLibPQ
    }
}
```

---

## Complete Data Flow Example: Simple SELECT

```sql
-- Input: examples/clickhouse/queries.sql
-- name: SelectUsers :many
SELECT * FROM users WHERE id = @user_id;
```

### Processing Steps:

1. **Read & Preprocess**
   ```
   Input: "SELECT * FROM users WHERE id = @user_id"
   After preprocess: "SELECT * FROM users WHERE id = ?"
   ```

2. **Parse (ClickHouse)**
   ```
   ClickHouse AST:
   - SelectQuery
     - TargetList: [A_Star]
     - FromClause: [TableRef("users")]
     - WhereClause: BinaryOp(ColumnRef("id"), "=", PlaceHolder)
   ```

3. **Convert to sqlc AST**
   ```
   ast.SelectStmt
   - TargetList: [A_Star]
   - FromClause: [RangeVar(name="users")]
   - WhereClause: A_Expr
   ```

4. **Extract Metadata**
   ```
   Name: "SelectUsers"
   Cmd: ":many"
   Named params: {user_id: ParamRef position}
   ```

5. **Infer Query (Draft Analysis)**
   ```
   Rewrite: @user_id → $1 in AST representation
   Find table "users" in catalog
   ```

6. **Expand Stars**
   ```
   Catalog has: users(id, name, email)
   Generated edit:
     Position: [star location]
     Text: "`id`, `name`, `email`"
   Applied query: "SELECT `id`, `name`, `email` FROM users WHERE id = ?"
   ```

7. **Analyze Query (Full)**
   ```
   Parameters:
     - $1: user_id → UInt32 (from catalog or inference)
   
   Columns:
     - id: UInt32, not null
     - name: String, nullable
     - email: String, nullable
   ```

8. **Map Go Types**
   ```
   id:    uint32                    (not null)
   name:  sql.NullString            (nullable)
   email: sql.NullString            (nullable)
   ```

9. **Generate Code**

   **models.go:**
   ```go
   type SelectUsersRow struct {
       ID    uint32
       Name  sql.NullString
       Email sql.NullString
   }
   ```

   **queries.sql.go:**
   ```go
   type SelectUsersParams struct {
       UserID uint32
   }
   
   type SelectUsersRow struct {
       ID    uint32
       Name  sql.NullString
       Email sql.NullString
   }
   
   const selectUsers = `-- name: SelectUsers :many
   SELECT id, name, email FROM users WHERE id = ?
   `
   
   func (q *Queries) SelectUsers(ctx context.Context, userID uint32) ([]SelectUsersRow, error) {
       rows, err := q.db.QueryContext(ctx, selectUsers, userID)
       // ... scan rows ...
   }
   ```

---

## Key Points for ClickHouse

### Parser Entry: `internal/engine/clickhouse/parse.go`

- Uses `github.com/AfterShip/clickhouse-sql-parser` for ClickHouse parsing
- Preprocesses named parameters to valid ClickHouse syntax
- Preserves location information for star expansion
- Converts ClickHouse AST to sqlc AST via `convert.go`

### Converter: `internal/engine/clickhouse/convert.go`

- Maps all ClickHouse statement types (SELECT, INSERT, CREATE TABLE, etc.)
- Recursively converts expressions (functions, operators, literals)
- Creates sqlc AST nodes that the rest of the pipeline understands

### Type Mapping: `internal/codegen/golang/clickhouse_type.go`

- ClickHouse-specific type mappings to Go
- Handles UInt/Int variants (unsigned integers)
- Supports nullable types with `sql.NullX` when needed
- Can use native ClickHouse driver (`clickhouse-go/v2`) for better null handling

### Catalog: `internal/engine/clickhouse/catalog.go`

- Builds table/column metadata from parsed schema
- Used for star expansion and parameter type inference

---

## Text Manipulation Points in sqlc

| Location | Purpose | Example |
|---|---|---|
| `parse.go:preprocessNamedParameters()` | Convert named params to `?` for parsing | `@user_id` → `?` |
| `expand.go:expand()` | Generate column list for `SELECT *` | `*` → `` `id`, `name`, `email` `` |
| `expand.go:quote()` | Quote identifiers based on DB | `name` → `` `name` `` (ClickHouse) |
| `rewrite.go:NamedParameters()` | Convert `?` back to named for codegen | `?` → `$1` (postgres) or `:1` (go template) |

---

## Summary

The query processing pipeline is:

1. **Parse** (database-specific parser)
2. **Convert** (to sqlc internal AST)
3. **Analyze** (extract types & columns)
4. **Expand** (star expansion, identifier quoting)
5. **Generate** (output code)

For ClickHouse, custom logic is localized to:
- `internal/engine/clickhouse/*` - Parsing and AST conversion
- `internal/codegen/golang/clickhouse_type.go` - Type mapping
- `internal/compiler/engine.go` - Engine registration
- `internal/compiler/expand.go` - Backtick quoting

The rest of the pipeline (analysis, code generation) works generically across all databases.
