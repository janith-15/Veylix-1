// Command outboxprobe is a READ-ONLY diagnostic that dumps the ten newest
// outbox rows with their full payload, so an operator can see exactly what
// Veylix queued for a 3X-UI panel, whether it was idempotency-keyed, and how it
// failed.
//
// It differs from cmd/ob on purpose: cmd/ob prints a narrow, terminal-safe
// projection (no payload), while this probe prints idempotency_key and
// payload_json because that is the evidence a stuck or ambiguous remote write
// needs. Treat its output as sensitive.
//
// Design rules (mirrors cmd/ob and cmd/panelprobe):
//   - it is a SEPARATE binary; the server never dumps its own outbox;
//   - it never writes: no UPDATE/INSERT/DELETE, no migration, no panel call —
//     the outbox is the only record of a remote write, so a diagnostic must not
//     be able to change its state;
//   - rows is always closed (defer) — a leaked *sql.Rows pins a connection out
//     of the pool, and with WAL enabled the next write waits on it;
//   - rows.Err() is checked after the loop. Next() returning false means "no
//     more rows" OR "the iteration stopped on an error", so without the check a
//     mid-stream failure (busy timeout, context cancel, I/O error) would print a
//     shorter list that looks like a complete answer — "the outbox is clean"
//     would then be a wrong conclusion, not just an incomplete one.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"veylix/backend/internal/config"
	"veylix/backend/internal/store"
)

// probeLimit is the number of newest rows this probe prints.
const probeLimit = 10

// outboxProbeRow is the projection this command prints.
type outboxProbeRow struct {
	ID             int64
	Op             string
	ServerID       int64
	IdempotencyKey string
	State          store.OutboxState
	Attempts       int
	LastError      string
	PayloadJSON    string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("outboxprobe failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	timeoutFlag := flag.Duration("timeout", 10*time.Second, "overall deadline for the read")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	// Read-only: the database is opened but NOT migrated. A diagnostic must not
	// change the schema of the database it is inspecting.
	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("database %s is not readable: %w (is VEYLIX_DATA_DIR correct?)", cfg.DBPath, err)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	rows, err := latestOutbox(ctx, st.DB())
	if err != nil {
		// A stream error is returned before anything is printed: a truncated
		// row list must never be presented as a complete answer.
		return err
	}
	printOutbox(os.Stdout, rows)
	return nil
}

// latestOutbox returns the ten newest outbox rows, newest first.
//
// COALESCE on last_error keeps a row with a NULL error scanable instead of
// failing the whole probe; the column is NOT NULL today, so the guard is for
// databases written by older schemas.
func latestOutbox(ctx context.Context, db *sql.DB) ([]outboxProbeRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, op, server_id, idempotency_key, state, attempts, COALESCE(last_error,''), payload_json
		   FROM outbox ORDER BY id DESC LIMIT ?`, probeLimit)
	if err != nil {
		return nil, fmt.Errorf("outboxprobe: list outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]outboxProbeRow, 0, probeLimit)
	for rows.Next() {
		var r outboxProbeRow
		if err := rows.Scan(&r.ID, &r.Op, &r.ServerID, &r.IdempotencyKey,
			&r.State, &r.Attempts, &r.LastError, &r.PayloadJSON); err != nil {
			return nil, fmt.Errorf("outboxprobe: scan outbox row: %w", err)
		}
		out = append(out, r)
	}
	// Next() stops on both "no more rows" and "an error occurred"; only Err()
	// separates the two, so the list is not a complete answer without it.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outboxprobe: iterate outbox: %w", err)
	}
	return out, nil
}

// printOutbox renders the rows as an aligned table. The payload is printed last
// because it is the only multi-line field a row may contain.
func printOutbox(w *os.File, rows []outboxProbeRow) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "outbox: no rows")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tOP\tSERVER\tSTATE\tATTEMPTS\tIDEMPOTENCY KEY\tLAST ERROR\tPAYLOAD")
	for _, r := range rows {
		lastErr := r.LastError
		if lastErr == "" {
			lastErr = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%d\t%s\t%d\t%s\t%s\t%s\n",
			r.ID, r.Op, r.ServerID, r.State, r.Attempts, r.IdempotencyKey, lastErr, r.PayloadJSON)
	}
	if err := tw.Flush(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Error("writing outbox table", "err", err)
	}
}
