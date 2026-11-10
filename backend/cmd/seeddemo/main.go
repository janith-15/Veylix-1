// Command seeddemo creates one account per role so a developer can sign in to
// every screen immediately.
//
// Design rules:
//   - it is a SEPARATE binary; the server never seeds anything by itself;
//   - it refuses to run unless VEYLIX_SEED=1, so a stray `go run` in a staging
//     or production checkout cannot create accounts;
//   - passwords are never compiled in: they come from .veylix-secrets/seed.json
//     (git-ignored) or VEYLIX_SEED_JSON;
//   - it is idempotent: an email that already exists is skipped, not updated,
//     so re-running cannot silently reset a real password.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
	"veylix/backend/internal/store"
)

type account struct {
	Role        string `json:"role"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	QuotaLimit  int    `json:"quota_limit,omitempty"`
}

// seedFile is the JSON shape of .veylix-secrets/seed.json.
type seedFile struct {
	Accounts []account `json:"accounts"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	fileFlag := flag.String("file", "", "JSON file with accounts (default: ../.veylix-secrets/seed.json)")
	dryRun := flag.Bool("dry-run", false, "print what would be created, change nothing")
	flag.Parse()

	if os.Getenv("VEYLIX_SEED") != "1" {
		return errors.New("refusing to run: set VEYLIX_SEED=1 to confirm this is a development environment")
	}

	accounts, err := loadAccounts(*fileFlag)
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		return errors.New("no accounts to seed (file has an empty `accounts` array)")
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if err := cfg.EnsureDataDir(); err != nil {
		return err
	}
	slog.Info("seeding", "db", cfg.DBPath, "accounts", len(accounts), "dry_run", *dryRun)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	created, skipped := 0, 0
	for _, a := range accounts {
		role := auth.Role(a.Role)
		if !role.Valid() {
			return fmt.Errorf("%s: unknown role %q", a.Email, a.Role)
		}
		exists, err := userExists(ctx, st, a.Email)
		if err != nil {
			return err
		}
		if exists {
			slog.Info("skip (already exists)", "email", a.Email, "role", a.Role)
			skipped++
			continue
		}
		if *dryRun {
			slog.Info("would create", "email", a.Email, "role", a.Role)
			continue
		}
		// Validate the password BEFORE touching the database so a bad seed file
		// fails loudly instead of half-creating accounts.
		if err := auth.ValidatePasswordPolicy(a.Password); err != nil {
			return fmt.Errorf("%s: %w", a.Email, err)
		}
		if _, err := insertAccount(ctx, st, a, role); err != nil {
			return fmt.Errorf("%s: %w", a.Email, err)
		}
		slog.Info("created", "email", a.Email, "role", a.Role)
		created++
	}

	slog.Info("done", "created", created, "skipped", skipped)
	if created > 0 && !*dryRun {
		slog.Info("sign in at http://localhost:5173/login with the seeded credentials")
	}
	return nil
}

// loadAccounts reads the seed file (or VEYLIX_SEED_JSON as an inline override).
func loadAccounts(path string) ([]account, error) {
	if inline := os.Getenv("VEYLIX_SEED_JSON"); inline != "" {
		var f seedFile
		if err := json.Unmarshal([]byte(inline), &f); err != nil {
			return nil, fmt.Errorf("VEYLIX_SEED_JSON: %w", err)
		}
		return f.Accounts, nil
	}
	if path == "" {
		path = os.Getenv("VEYLIX_SEED_FILE")
	}
	if path == "" {
		path = filepath.Join("..", ".veylix-secrets", "seed.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed file %s: %w (copy .veylix-secrets/seed.example.json)", path, err)
	}
	var f seedFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse seed file %s: %w", path, err)
	}
	return f.Accounts, nil
}

func userExists(ctx context.Context, st *store.Store, email string) (bool, error) {
	var n int
	err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE email = ?`, strings.ToLower(strings.TrimSpace(email))).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("lookup %s: %w", email, err)
	}
	return n > 0, nil
}

// insertAccount stores one account directly.
//
// It mirrors auth.Service.CreateUserByRole but skips the super-admin actor
// check, because the operator running this binary is the trust boundary. The
// password policy is still applied by auth.HashPassword.
func insertAccount(ctx context.Context, st *store.Store, a account, role auth.Role) (int64, error) {
	hash, err := auth.HashPassword(a.Password)
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	limit := 0
	if role == auth.RoleAdmin {
		limit = a.QuotaLimit
		if limit == 0 {
			limit = 25 // sensible reseller default
		}
	}
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, quota_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'ACTIVE', 1, ?, ?, ?)`,
		strings.ToLower(strings.TrimSpace(a.Email)), hash, a.DisplayName, string(role), limit, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()

	// Audit the seed so the trail explains where these accounts came from.
	ctxJSON, _ := json.Marshal(map[string]string{"role": string(role), "source": "seeddemo"})
	_, _ = st.DB().ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (NULL, 'user.seed', 'user', ?, 'OK', ?, ?)`,
		fmt.Sprint(id), string(ctxJSON), now)
	return id, nil
}
