package compiler

import (
	"testing"

	"github.com/sqlc-dev/sqlc/internal/config"
)

func TestNewCompilerClickHouse(t *testing.T) {
	conf := config.SQL{
		Engine: config.EngineClickHouse,
	}

	combo := config.CombinedSettings{
		Global: config.Config{},
	}

	c, err := NewCompiler(conf, combo)
	if err != nil {
		t.Fatalf("unexpected error creating ClickHouse compiler: %v", err)
	}

	if c.parser == nil {
		t.Error("expected parser to be set")
	}

	if c.catalog == nil {
		t.Error("expected catalog to be set")
	}

	if c.parser.CommentSyntax().Dash == false {
		t.Error("expected ClickHouse parser to support dash comments")
	}

	if c.parser.CommentSyntax().SlashStar == false {
		t.Error("expected ClickHouse parser to support slash-star comments")
	}
}
