package main

import (
	"context"
	"database/sql"
	"errors"
	"lensamity/internal/config"
	"lensamity/migrations"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbURL, err := config.LoadDB()
	if err != nil {
		slog.Error("environment variable is not set", "error", "DATABASE_URL")
		os.Exit(1)
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		slog.Error("opening database failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("closing database failed", "error", err)
			os.Exit(1)
		}
	}()

	ctxPing, close := context.WithTimeout(ctx, 5*time.Second)
	defer close()

	if err := db.PingContext(ctxPing); err != nil {
		slog.Error("pinging database failed", "error", err)
		os.Exit(1)
	}

	goose.SetBaseFS(migrations.SQLfs)

	slog.Info("starting migrations...")

	if err := goose.UpContext(ctx, db, "sql"); err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Warn("migrations aborted due to shutdown signal", "error", err)
		} else {
			slog.Error("migration failed", "error", err)
		}
		os.Exit(1)
	}

	slog.Info("migrations completed successfully")
}
