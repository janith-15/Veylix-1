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
// Design rules (docs/INSTALLATION_DESIGN.md Â§3, Â§5-P3):
//
//   - Every destructive command takes a backup first, or refuses.
//   - `--dry-run` is honoured by every mutation, not just the top-level one.
//   - A remote/observable effect is never claimed from local state alone: the
//     only proof that the service is up is a 200 from /api/ready.
//   - No output path can print a key, a token or database content.
//
// Architecture: this lives in cmd/, the wiring layer. It holds no business
// rules and imports only the stdlib-only internal/versioning leaf (for numeric version comparison) â€” it shells out to the documented
// system tools (systemctl, caddy) and talks to the local API over HTTP.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
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

// dispatchTable maps every accepted verb to its handler.
//
// It is a map rather than a switch so the menu has something to be tested
// against: menu_test.go asserts every item in controlMenu names a key that
// exists here. A switch would make that unaskable, and a menu item pointing at a
// missing verb is exactly the failure this guards.
var dispatchTable = map[string]func(paths, []string) error{
	"status":      cmdStatus,
	"doctor":      cmdDoctor,
	"versions":    cmdVersions,
	"update":      cmdUpdate,
	"downgrade":   cmdDowngrade,
	"rollback":    cmdRollback,
	"domain-set":  cmdDomainSet,
	"ports":       cmdPorts,
	"backup":      cmdBackup,
	"restore":     cmdRestore,
	"repair":      cmdRepair,
	"logs":        cmdLogs,
	"cert":        cmdCert,
	"reset-admin": cmdResetAdmin,
	"uninstall":   cmdUninstall,
}

// serviceVerbs are the systemd lifecycle verbs; each maps to cmdService with its
// own verb, so they are registered programmatically rather than four times over.
var serviceVerbs = []string{"start", "stop", "restart", "enable", "disable"}

func init() {
	for _, v := range serviceVerbs {
		verb := v // capture per iteration
		dispatchTable[verb] = func(p paths, args []string) error {
			return cmdService(p, args, verb)
		}
	}

	// `menu` is registered here, not in the map literal above, because it closes
	// the loop: dispatchTable -> cmdMenu -> showMenu -> menu -> runFromMenu ->
	// run -> dispatchTable. Go rejects that as an initialization cycle when it is
	// written in the literal. Assigning in init() breaks the cycle at the point
	// where the table already exists.
	//
	// It must still be IN the table: menu_test.go proves every menu item names a
	// dispatched verb, and "open the menu" is itself an item callers rely on.
	dispatchTable["menu"] = cmdMenu
}

// run dispatches a subcommand. Every command is explicit: an unknown word is a
// usage error, never a silent no-op.
func run(args []string) error {
	if len(args) == 0 {
		// No arguments is the operator sitting at a keyboard asking "what now?".
		// showMenu answers with the control menu, and falls back to the usage
		// text when stdin is a pipe so a script never blocks on a prompt.
		return showMenu("VEYLIX CONTROL MENU")
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
	case "menu":
		return showMenu("VEYLIX CONTROL MENU")
	}

	if fn, ok := dispatchTable[cmd]; ok {
		return fn(p, rest)
	}
	return usagef("unknown command %q", cmd)
}

func printUsage(w *os.File) { printUsageTo(w) }

// printUsageTo renders the usage text to any writer, so a test can assert that
// every dispatched command is documented.
func printUsageTo(w io.Writer) {
	fmt.Fprint(w, strings.TrimSpace(`
vpn-platformctl - Veylix operations CLI

Usage:
  vpn-platformctl                       control menu (interactive terminal)
  vpn-platformctl menu                  the same, spelled out
  vpn-platformctl <command> [flags]

Service:
  start                   start veylixd and confirm it became active
  stop                    stop veylixd and confirm it is inactive
  restart                 restart veylixd and confirm it answers /api/ready
  status                  show service, readiness and database state
  logs [-f] [-n N]        show the service log (journald; -f follows)
  enable                  start veylixd on boot
  disable                 do not start veylixd on boot

Maintain:
  doctor                  re-run every preflight check on this host
  versions                list every published release and mark the one running
  update                  upgrade to the latest release (backup first)
  downgrade <ver> --restore-backup <dir>
                          install an OLDER release; migrations are forward-only,
                          so the matching backup must be restored with it
  rollback                return to the previous binary / backup (backup first;
                          pass --skip-backup only if you mean to lose the
                          state being replaced)
  backup                  take a verified backup of the database and keys
  restore <dir>           restore a backup directory over the live data
  repair                  re-render config and reload; never touches data

Domain and access:
  domain-set <domain>     change the public domain (validate, reload, verify)
  domain-set <domain> --dns-provider cloudflare --cf-token <token>
                          issue the certificate through Cloudflare DNS-01 -
                          needs no free port, so it works beside 3X-UI
  ports                   show the HTTPS / HTTP / app ports in use
  ports set [--https-port N] [--http-port N] [--app-port N]
                          move the listeners; keeps any port you do not pass
  cert [--domain <fqdn>]  report the TLS certificate the domain actually serves
  reset-admin [--email <address>]
                          set a new Super Admin password (prompts, not echoed)
                          and revoke every session - the recovery path

Danger:
  uninstall [--purge-data] remove the service; data preserved by default,
                          --purge-data removes data, keys, backups, the Caddy
                          block and the service account too

Shared flags:
  --dry-run               print what would happen; change nothing
  --json                  machine-readable output where supported
  --base <url>            loopback origin to probe (default: the app port
                          recorded in veylix.env, normally http://127.0.0.1:8080)

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
	// The default probe origin follows the port the daemon actually binds.
	// A hardcoded :8080 would call a healthy service DOWN on any host installed
	// with --app-port (D-123).
	fs.StringVar(&o.base, "base", envOr("VEYLIX_BASE_URL", defaultBaseURL(defaultPaths().envFile())),
		"loopback origin to probe (default: http://127.0.0.1:<app-port from veylix.env>)")
	return o
}

// parseArgs parses once and returns the positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	if err := fs.Parse(args); err != nil {
		return nil, usagef("%v", err)
	}
	return fs.Args(), nil
}
