// Command vpn-platformctl is the permanent operations CLI for a Veylix
// installation.
//
// It exists because an installer is a one-time bootstrap: once a host is running,
// every subsequent change (a new release, a new domain, a backup, a rollback)
// must be doable without re-running `install.sh`. Writing that logic in Go rather
// than shell gives three things a shell copy cannot: it is versioned with the
// binary it manages, it is covered by the same `go test` gate, and it cannot
// drift from a second implementation of the same preflight checks.
//
// Design rules (docs/INSTALLATION_DESIGN.md §3, §5-P3):
//
//   - Every destructive command takes a backup first, or refuses.
//   - `--dry-run` is honoured by every mutation, not just the top-level one.
//   - A remote/observable effect is never claimed from local state alone: the
//     only proof that the service is up is a 200 from /api/ready.
//   - No output path can print a key, a token or database content.
//
// Architecture: this lives in cmd/, the wiring layer. It holds no business
// rules and imports only the stdlib-only internal/versioning leaf (for numeric version comparison) — it shells out to the documented
// system tools (systemctl, caddy) and talks to the local API over HTTP.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// version is set at build time:
//
//	go build -ldflags "-X main.version=v1.4.0"
var version = "dev"

// Exit codes an operator and a script can both rely on.
const (
	exitOK         = 0
	exitFailed     = 1 // anything that failed
	exitUsage      = 2 // bad arguments
	exitIncomplete = 3 // installed but not publicly served (e.g. ACME failed)
)

// paths is the fixed layout. Every value can be overridden by an environment
// variable so the CLI can be exercised against a throwaway tree in tests; the
// defaults are the documented production layout.
type paths struct {
	BinDir    string // where veylixd lives
	ConfigDir string // /etc/veylix
	DataDir   string // /var/lib/veylix
	BackupDir string // /var/backups/veylix
	UnitPath  string // the systemd unit file
	Caddyfile string // the Caddy config
	LockPath  string
}

func defaultPaths() paths {
	return paths{
		BinDir:    envOr("VEYLIX_BIN_DIR", "/usr/local/bin"),
		ConfigDir: envOr("VEYLIX_CONFIG_DIR", "/etc/veylix"),
		DataDir:   envOr("VEYLIX_DATA_DIR", "/var/lib/veylix"),
		BackupDir: envOr("VEYLIX_BACKUP_DIR", "/var/backups/veylix"),
		UnitPath:  envOr("VEYLIX_UNIT_PATH", "/etc/systemd/system/veylixd.service"),
		Caddyfile: envOr("VEYLIX_CADDYFILE", "/etc/caddy/Caddyfile"),
		LockPath:  envOr("VEYLIX_LOCK_PATH", "/run/veylix-install.lock"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (p paths) binary() string  { return p.BinDir + "/veylixd" }
func (p paths) prevBin() string { return p.BinDir + "/veylixd.prev" }
func (p paths) dbFile() string  { return p.DataDir + "/veylix.db" }
func (p paths) envFile() string { return p.ConfigDir + "/veylix.env" }
func (p paths) encKey() string  { return p.ConfigDir + "/enc.key" }
func (p paths) sessKey() string { return p.ConfigDir + "/session.key" }

// opt holds the flags every command shares.
type opt struct {
	dryRun bool
	json   bool
	base   string // the loopback origin to probe
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		var ue usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(os.Stderr, "vpn-platformctl: %v\n", ue.err)
			fmt.Fprintf(os.Stderr, "\nrun 'vpn-platformctl help' for usage\n")
			os.Exit(exitUsage)
		}
		var ie incompleteError
		if errors.As(err, &ie) {
			fmt.Fprintf(os.Stderr, "vpn-platformctl: %v\n", ie.err)
			os.Exit(exitIncomplete)
		}
		fmt.Fprintf(os.Stderr, "vpn-platformctl: %v\n", err)
		os.Exit(exitFailed)
	}
	os.Exit(exitOK)
}

type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }

type incompleteError struct{ err error }

func (i incompleteError) Error() string { return i.err.Error() }

func usagef(format string, args ...any) error {
	return usageError{err: fmt.Errorf(format, args...)}
}

// run dispatches a subcommand. Every command is explicit: an unknown word is a
// usage error, never a silent no-op.
func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}

	cmd := args[0]
	rest := args[1:]
	p := defaultPaths()

	switch cmd {
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	case "version", "--version", "-v":
		fmt.Printf("vpn-platformctl %s\n", version)
		return nil
	case "status":
		return cmdStatus(p, rest)
	case "doctor":
		return cmdDoctor(p, rest)
	case "versions":
		return cmdVersions(p, rest)
	case "update":
		return cmdUpdate(p, rest)
	case "rollback":
		return cmdRollback(p, rest)
	case "domain-set":
		return cmdDomainSet(p, rest)
	case "backup":
		return cmdBackup(p, rest)
	case "restore":
		return cmdRestore(p, rest)
	case "repair":
		return cmdRepair(p, rest)
	case "uninstall":
		return cmdUninstall(p, rest)
	default:
		return usagef("unknown command %q", cmd)
	}
}

func printUsage(w *os.File) {
	fmt.Fprint(w, strings.TrimSpace(`
vpn-platformctl - Veylix operations CLI

Usage:
  vpn-platformctl <command> [flags]

Commands:
  version                 print the CLI version
  status                  show service, readiness and database state
  doctor                  re-run every preflight check on this host
  versions                list every published release and mark the one running
  update                  upgrade to the latest release (backup first)
  rollback                return to the previous binary / backup
  domain-set <domain>     change the public domain (validate, reload, verify)
  backup                  take a verified backup of the database and keys
  restore <dir>           restore a backup directory over the live data
  repair                  re-render config and reload; never touches data
  uninstall [--purge-data] remove the service; data preserved by default

Shared flags:
  --dry-run               print what would happen; change nothing
  --json                  machine-readable output where supported
  --base <url>            loopback origin to probe (default http://127.0.0.1:8080)

Exit codes:
  0  success
  1  failed
  2  bad arguments
  3  installed but not publicly served (e.g. certificate issuance failed)

Nothing here prints a key, a token or database content.
`))
	fmt.Fprintln(w)
}

// newFlagSet returns a FlagSet that never prints its own error text: the caller
// turns a parse failure into a usageError so the help hint is consistent.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// addCommonFlags registers the shared flags. It deliberately does NOT parse:
// a command registers its own flags first, then calls parseArgs once. Parsing
// twice is how `flag provided but not defined` happens - the first parse runs
// before the command's own flags exist.
func addCommonFlags(fs *flag.FlagSet) *opt {
	o := &opt{}
	fs.BoolVar(&o.dryRun, "dry-run", false, "print what would happen; change nothing")
	fs.BoolVar(&o.json, "json", false, "machine-readable output where supported")
	fs.StringVar(&o.base, "base", envOr("VEYLIX_BASE_URL", "http://127.0.0.1:8080"),
		"loopback origin to probe")
	return o
}

// parseArgs parses once and returns the positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	if err := fs.Parse(args); err != nil {
		return nil, usagef("%v", err)
	}
	return fs.Args(), nil
}
