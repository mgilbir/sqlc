package clickhouse

import (
	"log"
	"strconv"
	"strings"

	chparser "github.com/AfterShip/clickhouse-sql-parser/parser"

	"github.com/sqlc-dev/sqlc/internal/debug"
	"github.com/sqlc-dev/sqlc/internal/sql/ast"
)

type cc struct {
	paramCount int
}

func todo(n chparser.Expr) *ast.TODO {
	if debug.Active {
		log.Printf("clickhouse.convert: Unsupported AST node type %T\n", n)
		log.Printf("clickhouse.convert: This node type may not be fully supported yet. Consider using different query syntax or filing an issue.\n")
	}
	return &ast.TODO{}
}

func identifier(id string) string {
	return strings.ToLower(id)
}

func NewIdentifier(t string) *ast.String {
	return &ast.String{Str: identifier(t)}
}

// convert converts a ClickHouse AST node to a sqlc AST node
func (c *cc) convert(node chparser.Expr) ast.Node {
	if node == nil {
		return nil
	}

	switch n := node.(type) {
	case *chparser.SelectQuery:
		return c.convertSelectQuery(n)
	case *chparser.InsertStmt:
		return c.convertInsertStmt(n)
	case *chparser.AlterTable:
		return c.convertAlterTable(n)
	case *chparser.CreateTable:
		return c.convertCreateTable(n)
	case *chparser.CreateDatabase:
		return c.convertCreateDatabase(n)
	case *chparser.DropStmt:
		return c.convertDropStmt(n)
	case *chparser.OptimizeStmt:
		return c.convertOptimizeStmt(n)
	case *chparser.DescribeStmt:
		return c.convertDescribeStmt(n)
	case *chparser.ExplainStmt:
		return c.convertExplainStmt(n)
	case *chparser.ShowStmt:
		return c.convertShowStmt(n)
	case *chparser.TruncateTable:
		return c.convertTruncateTable(n)

	// Expression nodes
	case *chparser.Ident:
		return c.convertIdent(n)
	case *chparser.ColumnExpr:
		return c.convertColumnExpr(n)
	case *chparser.FunctionExpr:
		return c.convertFunctionExpr(n)
	case *chparser.BinaryOperation:
		return c.convertBinaryOperation(n)
	case *chparser.NumberLiteral:
		return c.convertNumberLiteral(n)
	case *chparser.StringLiteral:
		return c.convertStringLiteral(n)
	case *chparser.QueryParam:
		return c.convertQueryParam(n)
	case *chparser.NestedIdentifier:
		return c.convertNestedIdentifier(n)
	case *chparser.OrderExpr:
		return c.convertOrderExpr(n)
	case *chparser.PlaceHolder:
		return c.convertPlaceHolder(n)
	case *chparser.JoinTableExpr:
		return c.convertJoinTableExpr(n)

	// Additional expression nodes
	case *chparser.CastExpr:
		return c.convertCastExpr(n)
	case *chparser.CaseExpr:
		return c.convertCaseExpr(n)
	case *chparser.WindowFunctionExpr:
		return c.convertWindowFunctionExpr(n)
	case *chparser.IsNullExpr:
		return c.convertIsNullExpr(n)
	case *chparser.IsNotNullExpr:
		return c.convertIsNotNullExpr(n)
	case *chparser.UnaryExpr:
		return c.convertUnaryExpr(n)
	case *chparser.MapLiteral:
		return c.convertMapLiteral(n)
	case *chparser.ParamExprList:
		return c.convertParamExprList(n)

	default:
		// Return TODO for unsupported node types
		return todo(n)
	}
}

func (c *cc) convertSelectQuery(stmt *chparser.SelectQuery) ast.Node {
	selectStmt := &ast.SelectStmt{
		TargetList:   c.convertSelectItems(stmt.SelectItems),
		FromClause:   c.convertFromClause(stmt.From),
		WhereClause:  c.convertWhereClause(stmt.Where),
		GroupClause:  c.convertGroupByClause(stmt.GroupBy),
		HavingClause: c.convertHavingClause(stmt.Having),
		SortClause:   c.convertOrderByClause(stmt.OrderBy),
		WithClause:   c.convertWithClause(stmt.With),
	}

	// Handle ARRAY JOIN by integrating it into the FROM clause
	if stmt.ArrayJoin != nil {
		selectStmt.FromClause = c.mergeArrayJoinIntoFrom(selectStmt.FromClause, stmt.ArrayJoin)
	}

	// Handle DISTINCT
	if stmt.HasDistinct {
		selectStmt.DistinctClause = &ast.List{Items: []ast.Node{}}
	}

	// Handle LIMIT
	if stmt.Limit != nil {
		selectStmt.LimitCount = c.convertLimitClause(stmt.Limit)
		if stmt.Limit.Offset != nil {
			selectStmt.LimitOffset = c.convert(stmt.Limit.Offset)
		}
	}

	// Handle UNION/EXCEPT
	if stmt.UnionAll != nil {
		selectStmt.Op = ast.Union
		selectStmt.All = true
		selectStmt.Larg = selectStmt
		selectStmt.Rarg = c.convertSelectQuery(stmt.UnionAll).(*ast.SelectStmt)
	} else if stmt.UnionDistinct != nil {
		selectStmt.Op = ast.Union
		selectStmt.All = false
		selectStmt.Larg = selectStmt
		selectStmt.Rarg = c.convertSelectQuery(stmt.UnionDistinct).(*ast.SelectStmt)
	} else if stmt.Except != nil {
		selectStmt.Op = ast.Except
		selectStmt.Larg = selectStmt
		selectStmt.Rarg = c.convertSelectQuery(stmt.Except).(*ast.SelectStmt)
	}

	return selectStmt
}

func (c *cc) convertSelectItems(items []*chparser.SelectItem) *ast.List {
	list := &ast.List{Items: []ast.Node{}}
	for _, item := range items {
		list.Items = append(list.Items, c.convertSelectItem(item))
	}
	return list
}

func (c *cc) convertSelectItem(item *chparser.SelectItem) *ast.ResTarget {
	var name *string
	if item.Alias != nil {
		aliasName := identifier(item.Alias.Name)
		name = &aliasName
	}

	return &ast.ResTarget{
		Name: name,
		Val:  c.convert(item.Expr),
		Location: int(item.Pos()),
	}
}

func (c *cc) convertFromClause(from *chparser.FromClause) *ast.List {
	if from == nil {
		return &ast.List{}
	}
	
	list := &ast.List{Items: []ast.Node{}}
	
	// From.Expr can be a TableExpr, JoinExpr, or other expression
	if from.Expr != nil {
		list.Items = append(list.Items, c.convertFromExpr(from.Expr))
	}
	
	return list
}

func (c *cc) convertFromExpr(expr chparser.Expr) ast.Node {
	if expr == nil {
		return &ast.TODO{}
	}

	switch e := expr.(type) {
	case *chparser.TableExpr:
		return c.convertTableExpr(e)
	case *chparser.JoinExpr:
		return c.convertJoinExpr(e)
	default:
		return c.convert(expr)
	}
}

func (c *cc) convertTableExpr(expr *chparser.TableExpr) ast.Node {
	if expr == nil {
		return &ast.TODO{}
	}

	// The Expr field contains the actual table reference
	var baseNode ast.Node
	
	if tableIdent, ok := expr.Expr.(*chparser.TableIdentifier); ok {
		baseNode = c.convertTableIdentifier(tableIdent)
	} else if selectQuery, ok := expr.Expr.(*chparser.SelectQuery); ok {
		// Subquery
		rangeSubselect := &ast.RangeSubselect{
			Subquery: c.convert(selectQuery),
		}
		if expr.Alias != nil {
			if aliasIdent, ok := expr.Alias.Alias.(*chparser.Ident); ok {
				rangeSubselect.Alias = &ast.Alias{
					Aliasname: &aliasIdent.Name,
				}
			}
		}
		return rangeSubselect
	} else {
		baseNode = c.convert(expr.Expr)
	}

	return baseNode
}

func (c *cc) convertTableIdentifier(ident *chparser.TableIdentifier) *ast.RangeVar {
	var schema *string
	var table *string

	if ident.Database != nil {
		dbName := identifier(ident.Database.Name)
		schema = &dbName
	}

	if ident.Table != nil {
		tableName := identifier(ident.Table.Name)
		table = &tableName
	}

	rangeVar := &ast.RangeVar{
		Schemaname: schema,
		Relname:    table,
		Location:   int(ident.Pos()),
	}

	return rangeVar
}

func (c *cc) convertJoinExpr(join *chparser.JoinExpr) ast.Node {
	// JoinExpr represents JOIN operations
	// Left and Right are the expressions being joined
	// Modifiers contains things like "LEFT", "RIGHT", "INNER", etc.
	// Constraints contains the ON clause expression
	
	joinNode := &ast.JoinExpr{
		Larg: c.convertFromExpr(join.Left),
		Rarg: c.convertFromExpr(join.Right),
	}

	// Determine join type from modifiers
	joinType := "JOIN"
	for _, mod := range join.Modifiers {
		modUpper := strings.ToUpper(mod)
		if modUpper == "LEFT" || modUpper == "RIGHT" || modUpper == "FULL" || modUpper == "INNER" {
			joinType = modUpper + " " + joinType
		}
	}
	joinNode.Jointype = c.parseJoinType(joinType)

	// Handle ON clause
	if join.Constraints != nil {
		joinNode.Quals = c.convert(join.Constraints)
	}

	return joinNode
}

func (c *cc) parseJoinType(joinType string) ast.JoinType {
	upperType := strings.ToUpper(joinType)
	switch {
	case strings.Contains(upperType, "LEFT"):
		return ast.JoinTypeLeft
	case strings.Contains(upperType, "RIGHT"):
		return ast.JoinTypeRight
	case strings.Contains(upperType, "FULL"):
		return ast.JoinTypeFull
	case strings.Contains(upperType, "INNER"):
		return ast.JoinTypeInner
	default:
		return ast.JoinTypeInner
	}
}

func (c *cc) convertWhereClause(where *chparser.WhereClause) ast.Node {
	if where == nil {
		return nil
	}
	return c.convert(where.Expr)
}

func (c *cc) convertGroupByClause(groupBy *chparser.GroupByClause) *ast.List {
	if groupBy == nil {
		return &ast.List{}
	}

	list := &ast.List{Items: []ast.Node{}}
	// GroupBy.Expr is a single expression which might be a comma-separated list
	if groupBy.Expr != nil {
		// Just convert the expression as-is
		// The parser should handle comma-separated lists internally
		list.Items = append(list.Items, c.convert(groupBy.Expr))
	}
	return list
}

func (c *cc) convertHavingClause(having *chparser.HavingClause) ast.Node {
	if having == nil {
		return nil
	}
	return c.convert(having.Expr)
}

func (c *cc) convertOrderByClause(orderBy *chparser.OrderByClause) *ast.List {
	if orderBy == nil {
		return &ast.List{}
	}

	list := &ast.List{Items: []ast.Node{}}
	
	// OrderBy.Items is a slice of Expr
	// For now, just convert each item directly
	for _, item := range orderBy.Items {
		list.Items = append(list.Items, c.convert(item))
	}
	
	return list
}

func (c *cc) convertLimitClause(limit *chparser.LimitClause) ast.Node {
	if limit == nil || limit.Limit == nil {
		return nil
	}
	return c.convert(limit.Limit)
}

func (c *cc) convertWithClause(with *chparser.WithClause) *ast.WithClause {
	if with == nil {
		return nil
	}

	list := &ast.List{Items: []ast.Node{}}
	for _, cte := range with.CTEs {
		list.Items = append(list.Items, c.convertCTE(cte))
	}

	return &ast.WithClause{
		Ctes:     list,
		Location: int(with.Pos()),
	}
}

func (c *cc) convertCTE(cte *chparser.CTEStmt) *ast.CommonTableExpr {
	if cte == nil {
		return nil
	}

	// Extract CTE name from Expr (should be an Ident)
	var cteName *string
	if ident, ok := cte.Expr.(*chparser.Ident); ok {
		name := identifier(ident.Name)
		cteName = &name
	}

	return &ast.CommonTableExpr{
		Ctename:   cteName,
		Ctequery:  c.convert(cte.Alias),
		Location:  int(cte.Pos()),
	}
}

func (c *cc) convertInsertStmt(stmt *chparser.InsertStmt) ast.Node {
	insert := &ast.InsertStmt{
		Relation:      c.convertTableExprToRangeVar(stmt.Table),
		Cols:          c.convertColumnNames(stmt.ColumnNames),
		ReturningList: &ast.List{},
	}

	// Handle VALUES
	if len(stmt.Values) > 0 {
		insert.SelectStmt = &ast.SelectStmt{
			FromClause:  &ast.List{},
			TargetList:  &ast.List{},
			ValuesLists: c.convertValues(stmt.Values),
		}
	}

	// Handle INSERT INTO ... SELECT
	if stmt.SelectExpr != nil {
		insert.SelectStmt = c.convert(stmt.SelectExpr)
	}

	return insert
}

func (c *cc) convertTableExprToRangeVar(expr chparser.Expr) *ast.RangeVar {
	if tableIdent, ok := expr.(*chparser.TableIdentifier); ok {
		return c.convertTableIdentifier(tableIdent)
	}
	if ident, ok := expr.(*chparser.Ident); ok {
		name := identifier(ident.Name)
		return &ast.RangeVar{
			Relname:  &name,
			Location: int(ident.Pos()),
		}
	}
	return &ast.RangeVar{}
}

func (c *cc) convertColumnNames(colNames *chparser.ColumnNamesExpr) *ast.List {
	if colNames == nil {
		return &ast.List{}
	}

	list := &ast.List{Items: []ast.Node{}}
	for _, col := range colNames.ColumnNames {
		// ColumnNames contains NestedIdentifier which has pointers
		// Convert to identifier string
		if col.Ident != nil {
			list.Items = append(list.Items, &ast.String{Str: identifier(col.Ident.Name)})
		}
		if col.DotIdent != nil {
			list.Items = append(list.Items, &ast.String{Str: identifier(col.DotIdent.Name)})
		}
	}
	return list
}

func (c *cc) convertValues(values []*chparser.AssignmentValues) *ast.List {
	list := &ast.List{Items: []ast.Node{}}
	for _, valueSet := range values {
		inner := &ast.List{Items: []ast.Node{}}
		for _, val := range valueSet.Values {
			inner.Items = append(inner.Items, c.convert(val))
		}
		list.Items = append(list.Items, inner)
	}
	return list
}

func (c *cc) convertCreateTable(stmt *chparser.CreateTable) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}

	// Extract table name
	var schema *string
	var table *string
	if stmt.Name != nil {
		if stmt.Name.Database != nil {
			dbName := identifier(stmt.Name.Database.Name)
			schema = &dbName
		}
		if stmt.Name.Table != nil {
			tableName := identifier(stmt.Name.Table.Name)
			table = &tableName
		}
	}
	
	// If no schema/database specified, the table name might be in Name.Table or Name.Database
	// In ClickHouse parser, a simple "users" goes into Database field, not Table
	if table == nil && stmt.Name != nil && stmt.Name.Database != nil {
		tableName := identifier(stmt.Name.Database.Name)
		table = &tableName
		schema = nil // No schema specified, will use default
	}

	// Build TableName for CreateTableStmt
	tableName := &ast.TableName{}
	if schema != nil {
		tableName.Schema = *schema
	}
	if table != nil {
		tableName.Name = *table
	}

	createStmt := &ast.CreateTableStmt{
		Name:        tableName,
		IfNotExists: stmt.IfNotExists,
	}

	// Convert columns from TableSchema
	if stmt.TableSchema != nil && len(stmt.TableSchema.Columns) > 0 {
		cols := []*ast.ColumnDef{}
		for _, col := range stmt.TableSchema.Columns {
			if colDef, ok := col.(*chparser.ColumnDef); ok {
				if converted, ok := c.convertColumnDef(colDef).(*ast.ColumnDef); ok {
					cols = append(cols, converted)
				}
			}
		}
		createStmt.Cols = cols
	}

	// Note: ClickHouse-specific features like ENGINE, ORDER BY, PARTITION BY, and SETTINGS
	// are not stored in sqlc's CreateTableStmt as it's designed for PostgreSQL compatibility.
	// These features are parsed but not preserved in the AST for now.
	// In a full ClickHouse implementation, we might extend CreateTableStmt or create
	// ClickHouse-specific statement types.

	return createStmt
}

func (c *cc) convertCreateDatabase(stmt *chparser.CreateDatabase) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}

	var schemaName string
	if stmt.Name != nil {
		// Name is usually an Ident
		if ident, ok := stmt.Name.(*chparser.Ident); ok {
			schemaName = identifier(ident.Name)
		}
	}

	return &ast.CreateSchemaStmt{
		Name:        &schemaName,
		IfNotExists: stmt.IfNotExists,
	}
}

func (c *cc) convertDropStmt(stmt *chparser.DropStmt) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}
	
	// ClickHouse DROP statements are mostly structural (DROP TABLE, DROP DATABASE)
	// sqlc doesn't have a dedicated DropStmt, so return TODO
	// This is expected - DROP is a DDL statement not typically used in application queries
	return &ast.TODO{}
}

func (c *cc) convertAlterTable(stmt *chparser.AlterTable) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}
	
	// ClickHouse uses ALTER TABLE for modifications that would be UPDATE/DELETE in other DBs
	// sqlc doesn't have dedicated support for ALTER TABLE modifications
	// This is expected - ALTER TABLE is DDL, not typically used in application queries
	return &ast.TODO{}
}

func (c *cc) convertOptimizeStmt(stmt *chparser.OptimizeStmt) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}
	
	// OPTIMIZE is a ClickHouse-specific statement for maintenance
	// Not a query statement that generates application code
	return &ast.TODO{}
}

func (c *cc) convertDescribeStmt(stmt *chparser.DescribeStmt) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}
	
	// DESCRIBE/DESC is a metadata query - useful for introspection but not
	// typically used in application code generation workflows
	return &ast.TODO{}
}

func (c *cc) convertExplainStmt(stmt *chparser.ExplainStmt) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}
	
	// EXPLAIN is for query analysis, not application code
	return &ast.TODO{}
}

func (c *cc) convertShowStmt(stmt *chparser.ShowStmt) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}
	
	// SHOW is an introspection statement for metadata queries
	// While it returns result sets, it's not typically code-generated
	// Treating as TODO for now as it's not a primary use case
	return &ast.TODO{}
}

func (c *cc) convertTruncateTable(stmt *chparser.TruncateTable) ast.Node {
	if stmt == nil {
		return &ast.TODO{}
	}
	
	// TRUNCATE is a DDL statement for deleting all rows from a table
	// While executable, it's not typically generated as application code
	// Treating as TODO for now as it's a maintenance operation
	return &ast.TODO{}
}

func (c *cc) convertIdent(id *chparser.Ident) ast.Node {
	// Convert identifier to sqlc's representation
	return &ast.String{
		Str: identifier(id.Name),
	}
}

func (c *cc) convertColumnExpr(col *chparser.ColumnExpr) ast.Node {
	// ColumnExpr wraps an expression (could be Ident, NestedIdentifier, etc.)
	// Just convert the underlying expression
	return c.convert(col.Expr)
}

func (c *cc) convertFunctionExpr(fn *chparser.FunctionExpr) ast.Node {
	// Convert function calls like COUNT(*), SUM(column), etc.
	funcName := identifier(fn.Name.Name)
	
	// Handle sqlc_* functions (converted from sqlc.* during preprocessing)
	// Normalize back to sqlc.* schema.function format for proper AST representation
	var schema string
	var baseFuncName string
	
	if strings.HasPrefix(funcName, "sqlc_") {
		schema = "sqlc"
		baseFuncName = strings.TrimPrefix(funcName, "sqlc_")
	} else {
		baseFuncName = funcName
	}
	
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
			Schema: schema,
			Name: baseFuncName,
		},
		Funcname: &ast.List{
			Items: []ast.Node{
				&ast.String{Str: funcName},
			},
		},
		Args:     args,
		Location: int(fn.Pos()),
	}
}

func (c *cc) convertBinaryOperation(op *chparser.BinaryOperation) ast.Node {
	// Convert binary operations like =, !=, <, >, AND, OR, etc.
	return &ast.A_Expr{
		Kind: ast.A_Expr_Kind(0), // Default kind
		Name: &ast.List{
			Items: []ast.Node{
				&ast.String{Str: string(op.Operation)},
			},
		},
		Lexpr:    c.convert(op.LeftExpr),
		Rexpr:    c.convert(op.RightExpr),
		Location: int(op.Pos()),
	}
}

func (c *cc) convertNumberLiteral(num *chparser.NumberLiteral) ast.Node {
	if num == nil || num.Literal == "" {
		return &ast.A_Const{
			Val:      &ast.Integer{Ival: 0},
			Location: 0,
		}
	}

	numStr := num.Literal

	// Try to parse as integer first
	if !strings.ContainsAny(numStr, ".eE") {
		// Integer literal
		if ival, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			return &ast.A_Const{
				Val:      &ast.Integer{Ival: ival},
				Location: int(num.Pos()),
			}
		}
	}

	// Try to parse as float
	if _, err := strconv.ParseFloat(numStr, 64); err == nil {
		return &ast.A_Const{
			Val:      &ast.Float{Str: numStr},
			Location: int(num.Pos()),
		}
	}

	// Fallback to integer 0 if parsing fails
	return &ast.A_Const{
		Val:      &ast.Integer{Ival: 0},
		Location: int(num.Pos()),
	}
}

func (c *cc) convertStringLiteral(str *chparser.StringLiteral) ast.Node {
	// The ClickHouse parser's StringLiteral.Pos() returns the position of the first
	// character after the opening quote. We need to adjust it to point to the opening
	// quote itself for correct location tracking in rewrite.NamedParameters, which uses
	// args[0].Pos() - 1 to find the opening paren position.
	pos := int(str.Pos())
	if pos > 0 {
		pos-- // Move from first char inside quote to the opening quote
	}
	return &ast.A_Const{
		Val: &ast.String{
			Str: str.Literal,
		},
		Location: pos,
	}
}

func (c *cc) convertQueryParam(param *chparser.QueryParam) ast.Node {
	// ClickHouse uses ? for parameters
	c.paramCount += 1
	return &ast.ParamRef{
		Number:   c.paramCount,
		Location: int(param.Pos()),
		Dollar:   false, // ClickHouse uses ? notation, not $1
	}
}

func (c *cc) convertNestedIdentifier(nested *chparser.NestedIdentifier) ast.Node {
	// NestedIdentifier represents things like "database.table" or "table.column"
	fields := &ast.List{Items: []ast.Node{}}
	
	if nested.Ident != nil {
		fields.Items = append(fields.Items, &ast.String{Str: identifier(nested.Ident.Name)})
	}
	if nested.DotIdent != nil {
		fields.Items = append(fields.Items, &ast.String{Str: identifier(nested.DotIdent.Name)})
	}

	return &ast.ColumnRef{
		Fields:   fields,
		Location: int(nested.Pos()),
	}
}

func (c *cc) convertColumnDef(col *chparser.ColumnDef) ast.Node {
	if col == nil {
		return &ast.TODO{}
	}

	// Extract column name
	var colName string
	if col.Name != nil {
		if col.Name.Ident != nil {
			colName = identifier(col.Name.Ident.Name)
		} else if col.Name.DotIdent != nil {
			colName = identifier(col.Name.DotIdent.Name)
		}
	}

	// Convert column type
	var typeName *ast.TypeName
	if col.Type != nil {
		typeName = c.convertColumnType(col.Type)
	}

	columnDef := &ast.ColumnDef{
		Colname:   colName,
		TypeName:  typeName,
		IsNotNull: col.NotNull != nil,
	}

	return columnDef
}

func (c *cc) convertColumnType(colType chparser.ColumnType) *ast.TypeName {
	if colType == nil {
		return &ast.TypeName{
			Name:  "text",
			Names: &ast.List{Items: []ast.Node{NewIdentifier("text")}},
		}
	}

	// Extract type name - ColumnType is an interface, get the string representation
	typeName := colType.Type()

	// Map ClickHouse types to PostgreSQL-compatible types for sqlc
	mappedType := mapClickHouseType(typeName)

	return &ast.TypeName{
		Name:  mappedType,
		Names: &ast.List{Items: []ast.Node{NewIdentifier(mappedType)}},
	}
}

// mapClickHouseType maps ClickHouse data types to PostgreSQL-compatible types
// that sqlc understands for Go code generation
func mapClickHouseType(chType string) string {
	chType = strings.ToLower(chType)
	
	switch {
	// Integer types (UInt variants - unsigned)
	case strings.HasPrefix(chType, "uint8"):
		return "uint8"
	case strings.HasPrefix(chType, "uint16"):
		return "uint16"
	case strings.HasPrefix(chType, "uint32"):
		return "uint32"
	case strings.HasPrefix(chType, "uint64"):
		return "uint64"
	// Integer types (Int variants - signed)
	case strings.HasPrefix(chType, "int8"):
		return "int8"
	case strings.HasPrefix(chType, "int16"):
		return "int16"
	case strings.HasPrefix(chType, "int32"):
		return "int32"
	case strings.HasPrefix(chType, "int64"):
		return "int64"
	case strings.HasPrefix(chType, "int128"):
		return "numeric"
	case strings.HasPrefix(chType, "int256"):
		return "numeric"
	
	// Float types
	case strings.HasPrefix(chType, "float32"):
		return "real"
	case strings.HasPrefix(chType, "float64"):
		return "double precision"
	
	// Decimal types
	case strings.HasPrefix(chType, "decimal"):
		return "numeric"
	
	// String types
	case chType == "string":
		return "text"
	case strings.HasPrefix(chType, "fixedstring"):
		return "varchar"
	
	// Date/Time types
	case chType == "date":
		return "date"
	case chType == "date32":
		return "date"
	case chType == "datetime":
		return "timestamp"
	case chType == "datetime64":
		return "timestamp"
	
	// Boolean
	case chType == "bool":
		return "boolean"
	
	// UUID
	case chType == "uuid":
		return "uuid"
	
	// Array types
	case strings.HasPrefix(chType, "array"):
		// Extract element type and make it an array
		// For now, just return text[] as a fallback
		return "text[]"
	
	// JSON types
	case strings.Contains(chType, "json"):
		return "jsonb"
	
	// Default fallback
	default:
		return "text"
	}
}

func (c *cc) convertOrderExpr(order *chparser.OrderExpr) ast.Node {
	if order == nil {
		return &ast.TODO{}
	}

	sortBy := &ast.SortBy{
		Node:     c.convert(order.Expr),
		Location: int(order.Pos()),
	}

	// Handle sort direction
	switch order.Direction {
	case "DESC":
		sortBy.SortbyDir = ast.SortByDirDesc
	case "ASC":
		sortBy.SortbyDir = ast.SortByDirAsc
	default:
		sortBy.SortbyDir = ast.SortByDirDefault
	}

	return sortBy
}

func (c *cc) convertPlaceHolder(ph *chparser.PlaceHolder) ast.Node {
	// PlaceHolder is ClickHouse's ? parameter
	c.paramCount += 1
	return &ast.ParamRef{
		Number:   c.paramCount,
		Location: int(ph.Pos()),
		Dollar:   false, // ClickHouse uses ? notation, not $1
	}
}

func (c *cc) convertJoinTableExpr(jte *chparser.JoinTableExpr) ast.Node {
	if jte == nil || jte.Table == nil {
		return &ast.TODO{}
	}
	// JoinTableExpr is a wrapper around TableExpr with optional modifiers
	// Just extract the underlying table expression
	return c.convertTableExpr(jte.Table)
}

// convertCastExpr converts CAST expressions like CAST(column AS type)
func (c *cc) convertCastExpr(castExpr *chparser.CastExpr) ast.Node {
	if castExpr == nil {
		return &ast.TODO{}
	}

	// Convert the expression to be cast
	expr := c.convert(castExpr.Expr)

	// Convert the target type - AsType is an Expr, need to extract type information
	var typeName *ast.TypeName
	if castExpr.AsType != nil {
		// The AsType is typically a ColumnExpr or Ident representing the type
		// We need to convert it to a TypeName
		if colType, ok := castExpr.AsType.(chparser.ColumnType); ok {
			typeName = c.convertColumnType(colType)
		} else if ident, ok := castExpr.AsType.(*chparser.Ident); ok {
			// Fallback: treat the identifier as a type name
			typeStr := identifier(ident.Name)
			typeName = &ast.TypeName{
				Name:  typeStr,
				Names: &ast.List{Items: []ast.Node{NewIdentifier(typeStr)}},
			}
		} else {
			typeName = &ast.TypeName{
				Name:  "text",
				Names: &ast.List{Items: []ast.Node{NewIdentifier("text")}},
			}
		}
	}

	return &ast.TypeCast{
		Arg:      expr,
		TypeName: typeName,
		Location: int(castExpr.Pos()),
	}
}

// convertCaseExpr converts CASE expressions
func (c *cc) convertCaseExpr(caseExpr *chparser.CaseExpr) ast.Node {
	if caseExpr == nil {
		return &ast.TODO{}
	}

	// Convert CASE input expression (if present)
	var arg ast.Node
	if caseExpr.Expr != nil {
		arg = c.convert(caseExpr.Expr)
	}

	// Convert WHEN clauses
	args := &ast.List{Items: []ast.Node{}}
	
	for _, when := range caseExpr.Whens {
		if when != nil {
			// Convert WHEN condition
			whenExpr := c.convert(when.When)
			args.Items = append(args.Items, whenExpr)

			// Convert THEN result
			thenExpr := c.convert(when.Then)
			args.Items = append(args.Items, thenExpr)
		}
	}

	// Convert ELSE clause (if present)
	var elseExpr ast.Node
	if caseExpr.Else != nil {
		elseExpr = c.convert(caseExpr.Else)
	}

	return &ast.CaseExpr{
		Arg:       arg,
		Args:      args,
		Defresult: elseExpr,
		Location:  int(caseExpr.Pos()),
	}
}

// convertWindowFunctionExpr converts window function expressions
func (c *cc) convertWindowFunctionExpr(winExpr *chparser.WindowFunctionExpr) ast.Node {
	if winExpr == nil {
		return &ast.TODO{}
	}

	// Convert the underlying function
	funcCall := c.convertFunctionExpr(winExpr.Function)

	// Convert OVER clause (OverExpr contains the window specification)
	var overClause *ast.WindowDef
	if winExpr.OverExpr != nil {
		// OverExpr might be a WindowExpr or other expression
		if winDef, ok := winExpr.OverExpr.(*chparser.WindowExpr); ok {
			overClause = c.convertWindowDef(winDef)
		}
	}

	// Wrap the function call in a window context
	if funcCall, ok := funcCall.(*ast.FuncCall); ok {
		funcCall.Over = overClause
		return funcCall
	}

	return funcCall
}

// convertWindowDef converts window definition
func (c *cc) convertWindowDef(winDef *chparser.WindowExpr) *ast.WindowDef {
	if winDef == nil {
		return nil
	}

	windowDef := &ast.WindowDef{
		Location: int(winDef.Pos()),
	}

	// Convert PARTITION BY
	if winDef.PartitionBy != nil && winDef.PartitionBy.Expr != nil {
		windowDef.PartitionClause = &ast.List{Items: []ast.Node{}}
		windowDef.PartitionClause.Items = append(windowDef.PartitionClause.Items, c.convert(winDef.PartitionBy.Expr))
	}

	// Convert ORDER BY
	if winDef.OrderBy != nil {
		windowDef.OrderClause = c.convertOrderByClause(winDef.OrderBy)
	}

	return windowDef
}

// convertIsNullExpr converts IS NULL expressions
func (c *cc) convertIsNullExpr(isNull *chparser.IsNullExpr) ast.Node {
	if isNull == nil {
		return &ast.TODO{}
	}

	return &ast.NullTest{
		Arg:          c.convert(isNull.Expr),
		Nulltesttype: ast.NullTestType(0), // IS_NULL = 0
		Location:     int(isNull.Pos()),
	}
}

// convertIsNotNullExpr converts IS NOT NULL expressions
func (c *cc) convertIsNotNullExpr(isNotNull *chparser.IsNotNullExpr) ast.Node {
	if isNotNull == nil {
		return &ast.TODO{}
	}

	return &ast.NullTest{
		Arg:          c.convert(isNotNull.Expr),
		Nulltesttype: ast.NullTestType(1), // IS_NOT_NULL = 1
		Location:     int(isNotNull.Pos()),
	}
}

// convertUnaryExpr converts unary expressions (like NOT, negation)
func (c *cc) convertUnaryExpr(unary *chparser.UnaryExpr) ast.Node {
	if unary == nil {
		return &ast.TODO{}
	}

	// Kind is a TokenKind (string)
	kindStr := string(unary.Kind)
	
	return &ast.A_Expr{
		Kind: ast.A_Expr_Kind(1), // AEXPR_OP_ANY or AEXPR_OP
		Name: &ast.List{
			Items: []ast.Node{
				&ast.String{Str: kindStr},
			},
		},
		Rexpr:    c.convert(unary.Expr),
		Location: int(unary.Pos()),
	}
}

// convertMapLiteral converts map/dictionary literals
func (c *cc) convertMapLiteral(mapLit *chparser.MapLiteral) ast.Node {
	if mapLit == nil {
		return &ast.TODO{}
	}

	// ClickHouse uses map literals like {'key': value, 'key2': value2}
	// Convert to a list of key-value pairs
	items := &ast.List{Items: []ast.Node{}}

	for _, kv := range mapLit.KeyValues {
		// Key is a StringLiteral value, need to convert it to a pointer
		keyLit := &kv.Key
		// Add key
		items.Items = append(items.Items, c.convert(keyLit))
		// Add value
		if kv.Value != nil {
			items.Items = append(items.Items, c.convert(kv.Value))
		}
	}

	// Return as a generic constant list (maps aren't directly supported in sqlc AST)
	return &ast.A_Const{
		Val:      items,
		Location: int(mapLit.Pos()),
	}
}

// convertParamExprList converts a parenthesized expression list to its content
// ParamExprList represents (expr1, expr2, ...) or (expr)
// We convert it by extracting and converting the items
func (c *cc) convertParamExprList(paramList *chparser.ParamExprList) ast.Node {
	if paramList == nil || paramList.Items == nil {
		return &ast.TODO{}
	}

	// If there's only one item, return that directly (unwrap the parens)
	if len(paramList.Items.Items) == 1 {
		return c.convert(paramList.Items.Items[0])
	}

	// If there are multiple items, convert them all and wrap in a list
	// This shouldn't normally happen in a WHERE clause, but handle it just in case
	items := &ast.List{Items: []ast.Node{}}
	for _, item := range paramList.Items.Items {
		if colExpr, ok := item.(*chparser.ColumnExpr); ok {
			items.Items = append(items.Items, c.convert(colExpr.Expr))
		} else {
			items.Items = append(items.Items, c.convert(item))
		}
	}
	return items
}

// mergeArrayJoinIntoFrom integrates ARRAY JOIN into the FROM clause as a special join
// ClickHouse's ARRAY JOIN is unique - it "unfolds" arrays into rows
// We represent it as a cross join with special handling
func (c *cc) mergeArrayJoinIntoFrom(fromClause *ast.List, arrayJoin *chparser.ArrayJoinClause) *ast.List {
	if fromClause == nil {
		fromClause = &ast.List{Items: []ast.Node{}}
	}

	// Convert the ARRAY JOIN expression to a join node
	arrayJoinNode := c.convertArrayJoinClause(arrayJoin)
	
	// Add the ARRAY JOIN to the FROM clause
	if arrayJoinNode != nil {
		fromClause.Items = append(fromClause.Items, arrayJoinNode)
	}

	return fromClause
}

// convertArrayJoinClause converts ClickHouse ARRAY JOIN to sqlc AST
// ARRAY JOIN unfolds arrays into rows - we represent it as a lateral join with array unnesting
func (c *cc) convertArrayJoinClause(arrayJoin *chparser.ArrayJoinClause) ast.Node {
	if arrayJoin == nil {
		return nil
	}

	// The Expr field contains the array expression(s) to unfold
	// It can be:
	// - A single column reference (e.g., "tags")
	// - A list of expressions with aliases (e.g., "ParsedParams AS pp")
	
	// Check if it's a ColumnExprList (multiple array expressions)
	if exprList, ok := arrayJoin.Expr.(*chparser.ColumnExprList); ok {
		// Multiple array expressions
		if len(exprList.Items) > 0 {
			// For now, handle the first item as the primary array join
			return c.convertArrayJoinItem(exprList.Items[0], arrayJoin.Type)
		}
	}
	
	// Single expression
	return c.convertArrayJoinItem(arrayJoin.Expr, arrayJoin.Type)
}

// convertArrayJoinItem converts a single ARRAY JOIN item
func (c *cc) convertArrayJoinItem(expr chparser.Expr, joinType string) ast.Node {
	if expr == nil {
		return nil
	}

	// Handle aliased expressions (e.g., "ParsedParams AS pp")
	if selectItem, ok := expr.(*chparser.SelectItem); ok {
		// Extract the expression and alias
		arrayExpr := c.convert(selectItem.Expr)
		
		var alias *ast.Alias
		if selectItem.Alias != nil {
			aliasName := identifier(selectItem.Alias.Name)
			alias = &ast.Alias{
				Aliasname: &aliasName,
			}
		}
		
		// Create a function call representing the array unnesting
		// We use a special function name "arrayJoin" to indicate this is an ARRAY JOIN
		funcCall := &ast.FuncCall{
			Func: &ast.FuncName{
				Name: "arrayjoin",
			},
			Args: &ast.List{
				Items: []ast.Node{arrayExpr},
			},
		}
		
		// Wrap in a RangeFunction to represent lateral unnesting
		rangeFunc := &ast.RangeFunction{
			Lateral: joinType == "LEFT", // LEFT ARRAY JOIN is lateral
			Functions: &ast.List{
				Items: []ast.Node{funcCall},
			},
			Alias: alias,
		}
		
		return rangeFunc
	}
	
	// Direct column reference without alias
	arrayExpr := c.convert(expr)
	
	funcCall := &ast.FuncCall{
		Func: &ast.FuncName{
			Name: "arrayjoin",
		},
		Args: &ast.List{
			Items: []ast.Node{arrayExpr},
		},
	}
	
	rangeFunc := &ast.RangeFunction{
		Lateral: joinType == "LEFT",
		Functions: &ast.List{
			Items: []ast.Node{funcCall},
		},
	}
	
	return rangeFunc
}
