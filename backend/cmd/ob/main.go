// Command ob is a READ-ONLY operator view of the outbox: it prints the most
// recent queued remote operations (op, state, attempts, last error) so an
// operator can see what Veylix tried to do on a 3X-UI panel and how it failed.
//
// Design rules (mirrors cmd/panelprobe and cmd/seeddemo):
//   - it is a SEPARATE binary; the server never prints its own outbox;
//   - it never writes: no UPDATE/INSERT/DELETE, no migration, no panel call —
//     the outbox is the only record of a remote write, so a diagnostic must not
//     be able to change its state;
//   - rows is always closed (defer) — a leaked *sql.Rows pins a connection out
//     of the pool, and with WAL enabled the next write waits on it;
//   - rows.Err() is checked after the loop. Next() returning false means "no
//     more rows" OR "the iteration stopped on an error", so without the check a
//     mid-stream failure (busy timeout, context cancel, I/O error) would print a
//     shorter list that looks like a complete answer — "no failed operations"
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

// outboxRow is the projection this command prints.
type outboxRow struct {
	ID        int64
	Op        string
	State     store.OutboxState
	Attempts  int
	LastError string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("ob failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	limitFlag := flag.Int("limit", 4, "how many of the newest outbox rows to show (1-100)")
	serverFlag := flag.Int64("server", 0, "only show rows of this integration id (0 = all)")
	timeoutFlag := flag.Duration("timeout", 10*time.Second, "overall deadline for the read")
	flag.Parse()

	if *limitFlag <= 0 || *limitFlag > 100 {
		return fmt.Errorf("-limit must be between 1 and 100 (got %d)", *limitFlag)
	}

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

	rows, err := latestOutbox(ctx, st.DB(), *serverFlag, *limitFlag)
	if err != nil {
		return err
	}
	printOutbox(os.Stdout, rows)
	return nil
}

// latestOutbox returns the newest outbox rows, newest first.
//
// The projection is deliberately narrow (id, op, state, attempts, last_error):
// payload_json and idempotency_key can carry panel client details, and this
// command prints to a terminal that may end up in a log or a screenshot.
// COALESCE on last_error keeps a row with a NULL error printable instead of
// failing the scan; the column is NOT NULL today, so the guard is for
// databases written by older schemas.
func latestOutbox(ctx context.Context, db *sql.DB, serverID int64, limit int) ([]outboxRow, error) {
	query := `SELECT id, op, state, attempts, COALESCE(last_error,'')
	            FROM outbox`
	var args []any
	if serverID > 0 {
		query += ` WHERE server_id = ?`
		args = append(args, serverID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ob: list outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]outboxRow, 0, limit)
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.Op, &r.State, &r.Attempts, &r.LastError); err != nil {
			return nil, fmt.Errorf("ob: scan outbox row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ob: iterate outbox: %w", err)
	}
	return out, nil
}

// printOutbox renders the rows as an aligned table and summarises the states,
// so a run that prints nothing is still an answer ("outbox is empty").
func printOutbox(w *os.File, rows []outboxRow) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "outbox: no rows")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tOP\tSTATE\tATTEMPTS\tLAST ERROR")
	states := make(map[store.OutboxState]int, len(rows))
	for _, r := range rows {
		lastErr := r.LastError
		if lastErr == "" {
			lastErr = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n", r.ID, r.Op, r.State, r.Attempts, lastErr)
		states[r.State]++
	}
	if err := tw.Flush(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Error("writing outbox table", "err", err)
	}

	// A fixed order keeps the summary diff-stable between runs.
	for _, s := range []store.OutboxState{
		store.OutboxPending, store.OutboxInFlight, store.OutboxRetry,
		store.OutboxFailed, store.OutboxApplied,
	} {
		if n := states[s]; n > 0 {
			fmt.Fprintf(w, "%s=%d ", s, n)
		}
	}
	fmt.Fprintf(w, "(showing %d newest)\n", len(rows))
}
