package clickhouse

import (
	"testing"

	"github.com/sqlc-dev/sqlc/internal/sql/ast"
)

func TestUpdateStatement(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr bool
		check   func(*testing.T, ast.Statement)
	}{
		{
			name:    "simple_update",
			sql:     "UPDATE users SET name = 'John' WHERE id = 1",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				if stmt.Raw == nil || stmt.Raw.Stmt == nil {
					t.Fatal("expected non-nil statement")
				}
				_, ok := stmt.Raw.Stmt.(*ast.UpdateStmt)
				if !ok {
					t.Fatalf("expected UpdateStmt, got %T", stmt.Raw.Stmt)
				}
			},
		},
		{
			name:    "update_multiple_columns",
			sql:     "UPDATE users SET name = 'Jane', email = 'jane@example.com' WHERE id = 2",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				updateStmt, ok := stmt.Raw.Stmt.(*ast.UpdateStmt)
				if !ok {
					t.Fatalf("expected UpdateStmt, got %T", stmt.Raw.Stmt)
				}
				// Check that we have at least 2 target items
				if updateStmt.TargetList == nil || len(updateStmt.TargetList.Items) < 2 {
					t.Error("expected at least 2 SET targets")
				}
			},
		},
		{
			name:    "update_with_complex_where",
			sql:     "UPDATE users SET status = 'active' WHERE id > 10 AND created_at > '2025-01-01'",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				updateStmt, ok := stmt.Raw.Stmt.(*ast.UpdateStmt)
				if !ok {
					t.Fatalf("expected UpdateStmt, got %T", stmt.Raw.Stmt)
				}
				if updateStmt.WhereClause == nil {
					t.Error("expected WHERE clause")
				}
			},
		},
		{
			name:    "update_with_limit",
			sql:     "UPDATE users SET verified = true LIMIT 5",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				updateStmt, ok := stmt.Raw.Stmt.(*ast.UpdateStmt)
				if !ok {
					t.Fatalf("expected UpdateStmt, got %T", stmt.Raw.Stmt)
				}
				// LIMIT clause in UPDATE is supported
				if updateStmt.LimitCount == nil {
					t.Error("expected LIMIT clause")
				}
			},
		},
		{
			name:    "update_with_from_clause",
			sql:     "UPDATE users SET status = 'active' FROM logs WHERE users.id = logs.user_id AND logs.action = 'verified'",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				updateStmt, ok := stmt.Raw.Stmt.(*ast.UpdateStmt)
				if !ok {
					t.Fatalf("expected UpdateStmt, got %T", stmt.Raw.Stmt)
				}
				// ClickHouse allows FROM in UPDATE
				if updateStmt.FromClause == nil || len(updateStmt.FromClause.Items) == 0 {
					t.Error("expected FROM clause")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parser := NewParser()
			stmts, err := testParseSQL(parser, tc.sql)

			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}

			if err == nil && len(stmts) > 0 {
				tc.check(t, stmts[0])
			}
		})
	}
}

func TestDeleteStatement(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr bool
		check   func(*testing.T, ast.Statement)
	}{
		{
			name:    "simple_delete",
			sql:     "DELETE FROM users WHERE id = 1",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				if stmt.Raw == nil || stmt.Raw.Stmt == nil {
					t.Fatal("expected non-nil statement")
				}
				_, ok := stmt.Raw.Stmt.(*ast.DeleteStmt)
				if !ok {
					t.Fatalf("expected DeleteStmt, got %T", stmt.Raw.Stmt)
				}
			},
		},
		{
			name:    "delete_with_complex_where",
			sql:     "DELETE FROM users WHERE id > 100 AND status = 'deleted' AND created_at < '2024-01-01'",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				deleteStmt, ok := stmt.Raw.Stmt.(*ast.DeleteStmt)
				if !ok {
					t.Fatalf("expected DeleteStmt, got %T", stmt.Raw.Stmt)
				}
				if deleteStmt.WhereClause == nil {
					t.Error("expected WHERE clause")
				}
			},
		},
		{
			name:    "delete_with_limit",
			sql:     "DELETE FROM logs WHERE timestamp < now() - interval 30 day LIMIT 1000",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				deleteStmt, ok := stmt.Raw.Stmt.(*ast.DeleteStmt)
				if !ok {
					t.Fatalf("expected DeleteStmt, got %T", stmt.Raw.Stmt)
				}
				if deleteStmt.LimitCount == nil {
					t.Error("expected LIMIT clause")
				}
			},
		},
		{
			name:    "delete_with_using_clause",
			sql:     "DELETE FROM comments USING posts WHERE comments.post_id = posts.id AND posts.status = 'archived'",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				deleteStmt, ok := stmt.Raw.Stmt.(*ast.DeleteStmt)
				if !ok {
					t.Fatalf("expected DeleteStmt, got %T", stmt.Raw.Stmt)
				}
				if deleteStmt.UsingClause == nil || len(deleteStmt.UsingClause.Items) == 0 {
					t.Error("expected USING clause")
				}
				if deleteStmt.LimitCount == nil {
					t.Error("expected LIMIT clause")
				}
			},
		},
		{
			name:    "delete_with_subquery_in_where",
			sql:     "DELETE FROM comments WHERE post_id IN (SELECT id FROM posts WHERE archived = true)",
			wantErr: false,
			check: func(t *testing.T, stmt ast.Statement) {
				deleteStmt, ok := stmt.Raw.Stmt.(*ast.DeleteStmt)
				if !ok {
					t.Fatalf("expected DeleteStmt, got %T", stmt.Raw.Stmt)
				}
				if deleteStmt.WhereClause == nil {
					t.Error("expected WHERE clause with subquery")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parser := NewParser()
			stmts, err := testParseSQL(parser, tc.sql)

			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}

			if err == nil && len(stmts) > 0 {
				tc.check(t, stmts[0])
			}
		})
	}
}

func TestUpdateDeleteWithNamedParameters(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "update_with_named_param",
			sql:  "UPDATE users SET verified = true WHERE id = sqlc.arg('user_id')",
		},
		{
			name: "delete_with_named_param",
			sql:  "DELETE FROM users WHERE email = sqlc.arg('email')",
		},
		{
			name: "update_with_multiple_named_params",
			sql:  "UPDATE users SET name = sqlc.arg('name'), email = sqlc.arg('email') WHERE id = sqlc.arg('id')",
		},
		{
			name: "delete_with_nullable_param",
			sql:  "DELETE FROM events WHERE (sqlc.narg('status') IS NULL OR status = sqlc.narg('status'))",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parser := NewParser()
			stmts, err := testParseSQL(parser, tc.sql)

			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			if len(stmts) == 0 {
				t.Fatal("expected at least one statement")
			}

			stmt := stmts[0]
			if stmt.Raw == nil || stmt.Raw.Stmt == nil {
				t.Fatal("expected non-nil statement")
			}
		})
	}
}

// testParseSQL is a helper to parse SQL using the ClickHouse parser
func testParseSQL(parser *Parser, sql string) ([]ast.Statement, error) {
	return parser.Parse(newStringReader(sql))
}

// newStringReader creates a reader from a string for testing
func newStringReader(s string) *stringReader {
	return &stringReader{data: []byte(s), pos: 0}
}

type stringReader struct {
	data []byte
	pos  int
}

func (sr *stringReader) Read(p []byte) (n int, err error) {
	if sr.pos >= len(sr.data) {
		return 0, nil // EOF
	}
	n = copy(p, sr.data[sr.pos:])
	sr.pos += n
	return n, nil
}
