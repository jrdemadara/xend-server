package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"xend.chat/m/migrations"
)

const usage = `usage: xend-migrate <command>

Commands:
  up      run all pending migrations
  down    roll back one migration
  status  print migration status
  reset   roll back all migrations
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "xend-migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return errors.New(strings.TrimSpace(usage))
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}

	switch args[0] {
	case "up":
		return goose.Up(db, migrations.Dir)
	case "down":
		return goose.Down(db, migrations.Dir)
	case "status":
		return goose.Status(db, migrations.Dir)
	case "reset":
		return goose.Reset(db, migrations.Dir)
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], strings.TrimSpace(usage))
	}
}
