// Command dbexec runs ONE interactive-free SQL statement against the Veylix
// SQLite database.
//
// It is a DEVELOPMENT/OPS tool, not part of the server: it exists so a smoke
// test or a recovery procedure can inspect and clean up rows without installing
// a sqlite3 client on the target machine (the same reason cmd/ob and cmd/panelprobe
// are separate binaries).
//
// Safety rules:
//   - it requires VEYLIX_DBEXEC=1, so a production shell cannot run it by accident;
//   - it refuses to run while the server is likely to be serving: the caller must
//     stop the server first (SQLite WAL allows it, but a concurrent write during a
//     cleanup is how a test corrupts real data);
//   - it prints the number of affected rows, never row contents, so a mistake is
//     visible in the output but no client detail leaks into a terminal log;
//   - it does NOT migrate: it must not change a schema it was asked to clean.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"veylix/backend/internal/config"
	"veylix/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dbexec: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	timeoutFlag := flag.Duration("timeout", 15*time.Second, "overall deadline for the statement")
	flag.Parse()

	if os.Getenv("VEYLIX_DBEXEC") != "1" {
		return fmt.Errorf("refusing to run: set VEYLIX_DBEXEC=1 (this is an ops tool, never part of the server)")
	}
	sqlText := flag.Arg(0)
	if sqlText == "" {
		return fmt.Errorf("pass exactly one SQL statement as the first argument")
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("database %s is not readable: %w", cfg.DBPath, err)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	res, err := st.DB().ExecContext(ctx, sqlText)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	fmt.Printf("ok: %d row(s) affected\n", n)
	return nil
}
