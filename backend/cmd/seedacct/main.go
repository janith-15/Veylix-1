// Command seedacct creates — or resets the password of — ONE staff account, so
// the three dev personas can be tested against real rows.
//
// Why a separate binary:
//   - `seeddemo` seeds the whole .veylix-secrets/seed.json file and SKIPS any
//     e-mail that already exists, which is right for a fresh checkout but leaves
//     no way to add the second/third admin a role test needs, or to fix a
//     password that drifted from the seed file;
//   - it must never be reachable from the server itself (Bible §14.2: accounts
//     are created by an operator, never by the running process).
//
// Safety rails (a mistyped command in production must fail, not write):
//   - VEYLIX_SEED=1 is required, exactly like seeddemo;
//   - the bind address must be LOOPBACK, so a staging/production .env (a real
//     host:port) refuses even if someone exports VEYLIX_SEED by accident;
//   - the password policy is the same one the API enforces (auth.HashPassword);
//   - a plain run is a no-op: an existing account is skipped unless
//     --reset-password is given;
//   - every write is audited (`user.seed` with source=seedacct).
//
// Usage:
//
//	go run ./cmd/seedacct --role admin --email a@b.test --name "A"          # password from seed.json
//	go run ./cmd/seedacct --role admin --email a@b.test --password "..." --reset-password
//	go run ./cmd/seedacct --role super_admin --dry-run
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
	"veylix/backend/internal/store"
)

// account mirrors one entry of .veylix-secrets/seed.json.
type account struct {
	Role        string `json:"role"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	QuotaLimit  int    `json:"quota_limit,omitempty"`
}

type seedFile struct {
	Accounts []account `json:"accounts"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("seedacct failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	roleFlag := flag.String("role", "", "account role: super_admin | admin")
	emailFlag := flag.String("email", "", "account e-mail (default: the seeded account for the role)")
	nameFlag := flag.String("name", "", "display name (default: from seed.json)")
	passFlag := flag.String("password", "", "password (default: from .veylix-secrets/seed.json)")
	quotaFlag := flag.Int("quota", 0, "reseller quota ceiling (admin only; default 25)")
	fileFlag := flag.String("file", "", "seed file to read defaults from (default ../.veylix-secrets/seed.json)")
	resetFlag := flag.Bool("reset-password", false, "rotate the password of an account that already exists")
	dryRun := flag.Bool("dry-run", false, "print what would happen, change nothing")
	flag.Parse()

	if os.Getenv("VEYLIX_SEED") != "1" {
		return errors.New("refusing to run: set VEYLIX_SEED=1 to confirm this is a development environment")
	}

	// The DB path comes from config, exactly as the server resolves it, so this
	// tool and the server can never disagree about which file they are editing.
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if !isLoopbackAddr(cfg.HTTPAddr) {
		return fmt.Errorf("refusing to run: VEYLIX_ADDR=%q is not a loopback address "+
			"(this tool only edits a local development database)", cfg.HTTPAddr)
	}

	acct, role, err := resolveAccount(*roleFlag, *emailFlag, *nameFlag, *passFlag, *quotaFlag, *fileFlag)
	if err != nil {
		return err
	}
	slog.Info("target", "role", string(role), "email", acct.Email, "db", cfg.DBPath, "dry_run", *dryRun)

	hash, err := auth.HashPassword(acct.Password)
	if err != nil {
		// Validate BEFORE touching the database: a rejected password must not
		// leave a half-created account behind.
		return fmt.Errorf("password rejected by policy: %w", err)
	}

	if err := cfg.EnsureDataDir(); err != nil {
		return err
	}

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
	return apply(ctx, st, acct, role, hash, *resetFlag, *dryRun)
}

// apply is the write decision, separated from flag parsing so it is obvious that
// nothing is written before the password has been validated.
func apply(ctx context.Context, st *store.Store, acct account, role auth.Role, hash string, reset, dryRun bool) error {
	id, exists, err := findUser(ctx, st, acct.Email)
	if err != nil {
		return err
	}

	switch {
	case exists && !reset:
		slog.Info("skip (already exists)", "email", acct.Email, "id", id,
			"hint", "pass --reset-password to rotate the password")
		return nil
	case exists:
		if dryRun {
			slog.Info("would reset password + role", "email", acct.Email, "id", id, "role", string(role))
			return nil
		}
		if err := updateAccount(ctx, st, id, acct, role, hash); err != nil {
			return err
		}
		slog.Info("password reset", "email", acct.Email, "id", id, "role", string(role))
		return nil
	}

	if dryRun {
		slog.Info("would create", "email", acct.Email, "role", string(role), "display_name", acct.DisplayName)
		return nil
	}
	newID, err := insertAccount(ctx, st, acct, role, hash)
	if err != nil {
		return err
	}
	slog.Info("created", "email", acct.Email, "id", newID, "role", string(role))
	slog.Info("sign in at http://localhost:5173/login with this e-mail and password")
	return nil
}

// resolveAccount fills any missing flag from the seed file, so the common case
// needs no password on the command line (a password in shell history, a process
// list or a chat log is a leak).
func resolveAccount(roleFlag, emailFlag, nameFlag, passFlag string, quotaFlag int, fileFlag string) (account, auth.Role, error) {
	role := auth.Role(strings.ToLower(strings.TrimSpace(roleFlag)))
	if role == "" {
		role = auth.RoleAdmin
	}
	if role == auth.RoleClient {
		return account{}, "", errors.New("clients self-register at /register; this tool mints staff accounts only")
	}
	if !role.Valid() {
		return account{}, "", fmt.Errorf("unknown role %q (want super_admin or admin)", roleFlag)
	}

	seeded, seedErr := loadSeed(fileFlag)
	if seedErr != nil && (passFlag == "" || emailFlag == "") {
		// A missing seed file must not block an explicit --email + --password.
		return account{}, "", seedErr
	}

	acct := account{Role: string(role), Email: emailFlag, DisplayName: nameFlag, Password: passFlag, QuotaLimit: quotaFlag}
	if match := pick(seeded, role); match != nil {
		if acct.Email == "" {
			acct.Email = match.Email
		}
		if acct.DisplayName == "" {
			acct.DisplayName = match.DisplayName
		}
		if acct.Password == "" {
			acct.Password = match.Password
		}
		if acct.QuotaLimit == 0 {
			acct.QuotaLimit = match.QuotaLimit
		}
	}
	if strings.TrimSpace(acct.Email) == "" {
		return account{}, "", errors.New("no e-mail: pass --email or add an account for this role to seed.json")
	}
	if acct.Password == "" {
		return account{}, "", errors.New("no password: pass --password or add an account for this role to seed.json")
	}
	if role == auth.RoleAdmin && acct.QuotaLimit == 0 {
		acct.QuotaLimit = 25 // same sensible reseller default as seeddemo
	}
	return acct, role, nil
}

// pick returns the first seeded account for role, or nil.
func pick(accounts []account, role auth.Role) *account {
	for i := range accounts {
		if auth.Role(accounts[i].Role) == role {
			return &accounts[i]
		}
	}
	return nil
}

// loadSeed reads the seed file (or VEYLIX_SEED_JSON) — the same sources seeddemo
// uses, so both tools agree about the demo credentials.
func loadSeed(path string) ([]account, error) {
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
		return nil, fmt.Errorf("read seed file %s: %w", path, err)
	}
	var f seedFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse seed file %s: %w", path, err)
	}
	return f.Accounts, nil
}

// findUser reports whether the e-mail exists and its id.
func findUser(ctx context.Context, st *store.Store, email string) (int64, bool, error) {
	var id int64
	err := st.DB().QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("lookup %s: %w", email, err)
	}
	return id, true, nil
}

// insertAccount writes one staff account and audits where it came from.
func insertAccount(ctx context.Context, st *store.Store, a account, role auth.Role, hash string) (int64, error) {
	now := time.Now().Unix()
	limit := 0
	if role == auth.RoleAdmin {
		limit = a.QuotaLimit
	}
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, quota_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'ACTIVE', 1, ?, ?, ?)`,
		strings.ToLower(strings.TrimSpace(a.Email)), hash, a.DisplayName, string(role), limit, now, now)
	if err != nil {
		return 0, fmt.Errorf("insert %s: %w", a.Email, err)
	}
	id, _ := res.LastInsertId()
	auditSeed(ctx, st, id, "seedacct.create", role)
	return id, nil
}

// updateAccount rotates the password and re-states role/name/quota, so a reset
// also repairs a row whose role drifted from the seed file.
func updateAccount(ctx context.Context, st *store.Store, id int64, a account, role auth.Role, hash string) error {
	now := time.Now().Unix()
	limit := 0
	if role == auth.RoleAdmin {
		limit = a.QuotaLimit
	}
	_, err := st.DB().ExecContext(ctx,
		`UPDATE users SET password_hash = ?, role = ?, display_name = COALESCE(NULLIF(?, ''), display_name),
		        quota_limit = ?, status = 'ACTIVE', updated_at = ?
		  WHERE id = ?`,
		hash, string(role), a.DisplayName, limit, now, id)
	if err != nil {
		return fmt.Errorf("update %s: %w", a.Email, err)
	}
	// A rotation must also end every live session: a cookie issued under the old
	// password would otherwise outlive it (the same rule as ChangePassword).
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return fmt.Errorf("revoke sessions for %s: %w", a.Email, err)
	}
	auditSeed(ctx, st, id, "seedacct.reset", role)
	return nil
}

// auditSeed records the write. A failed audit is reported, never silent, but it
// does not undo an account that already exists.
func auditSeed(ctx context.Context, st *store.Store, id int64, action string, role auth.Role) {
	ctxJSON, _ := json.Marshal(map[string]string{"role": string(role), "source": "seedacct", "action": action})
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (NULL, 'user.seed', 'user', ?, 'OK', ?, ?)`,
		strconv.FormatInt(id, 10), string(ctxJSON), time.Now().Unix()); err != nil {
		slog.Warn("audit write failed", "err", err, "user_id", id)
	}
}

// isLoopbackAddr reports whether the configured bind address is loopback-only.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
