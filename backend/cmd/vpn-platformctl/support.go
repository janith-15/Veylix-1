package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// out is every user-facing line the CLI prints. One function means one place to
// guarantee no secret is ever written.
func out(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// step prints a numbered phase banner so a failure names the phase that died.
var stepN int

func step(format string, args ...any) {
	stepN++
	fmt.Printf("\n==> [%d] %s\n", stepN, fmt.Sprintf(format, args...))
}

func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vpn-platformctl: WARNING: %s\n", fmt.Sprintf(format, args...))
}

// skipf prints a skipped check. A skip is never a pass: release-readiness
// Stage 5b requires the reason on screen.
func skipf(name, reason string) {
	fmt.Fprintf(os.Stderr, "vpn-platformctl: SKIPPED: %s - %s\n", name, reason)
}

// ---------------------------------------------------------------------------
// Process execution
//
// Every mutation goes through run() so --dry-run covers all of it. A command
// invoked directly with exec.Command elsewhere defeats --dry-run and is a
// review defect.
// ---------------------------------------------------------------------------

// runner executes external commands, or records them when dryRun is set.
type runner struct {
	dryRun bool
	// backupRoot is the backup root the environment is configured with. It is
	// carried here so a parsed backup path can be checked against the root the
	// script actually writes to, instead of "any absolute path".
	backupRoot string
}

// run executes name with args. Output is captured and only shown on failure, so
// a normal run is not a wall of systemctl noise.
func (r runner) run(name string, args ...string) (string, error) {
	if r.dryRun {
		out("[dry-run] %s", redactCmd(name, args))
		return "", nil
	}
	cmd := exec.Command(name, args...)
	combined, err := cmd.CombinedOutput()
	text := string(combined)
	if err != nil {
		return text, fmt.Errorf("%s: %w\n%s", redactCmd(name, args), err, strings.TrimSpace(text))
	}
	return text, nil
}

// have reports whether a command exists on PATH.
func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// redactCmd renders a command line for display. It exists so a future caller
// that passes a token as an argument cannot leak it into a log.
func redactCmd(name string, args []string) string {
	safe := make([]string, len(args))
	for i, a := range args {
		safe[i] = redact(a)
	}
	return strings.TrimSpace(name + " " + strings.Join(safe, " "))
}

// redact strips the secret shapes this product owns. Anything that looks like a
// 32-byte base64 key, a bearer token or a Google client secret becomes
// <redacted> before it can reach a terminal or a log.
//
// It works on the whole string rather than whitespace-separated words, because a
// key is very often glued to its name: `VEYLIX_ENCRYPTION_KEY=<44 chars>` is one
// word, and a word-splitting redactor misses it entirely.
func redact(s string) string {
	s = envKeyRe.ReplaceAllString(s, "${1}=<redacted>")
	s = bearerRe.ReplaceAllString(s, "${1} <redacted>")
	s = gocspxRe.ReplaceAllString(s, "<redacted>")
	s = b64KeyRe.ReplaceAllString(s, "<redacted>")
	return s
}

var (
	envKeyRe = regexp.MustCompile(`(VEYLIX_[A-Z_]*KEY)=[^\s"']+`)
	bearerRe = regexp.MustCompile(`(?i)(Bearer)\s+[A-Za-z0-9._\-]+`)
	gocspxRe = regexp.MustCompile(`GOCSPX-[A-Za-z0-9_\-]+`)
	b64KeyRe = regexp.MustCompile(`(^|[\s"'=])[A-Za-z0-9+/]{42,44}={0,2}($|[\s"',])`)
)

// fingerprint returns the first 8 hex characters of a file's SHA-256, or
// "absent". The key itself is never read into output.
func fingerprint(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "absent"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unreadable"
	}
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// fileExists reports whether anything (file or directory) exists at path. The
// branch logic asks "does state exist here", not "is it a regular file".
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// dirSizeMB reports the total size of the files in dir, in whole megabytes.
func dirSizeMB(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	return total / (1024 * 1024)
}

// ---------------------------------------------------------------------------
// Readiness
//
// The only honest proof that the service is up is a 200 from /api/ready. Local
// state (systemctl is-active, a process in ps) is reported but never treated as
// sufficient on its own.
// ---------------------------------------------------------------------------

// readyReport mirrors the fields GET /api/ready returns.
type readyReport struct {
	Status        string `json:"status"`
	SchemaVersion int    `json:"schema_version"`
	UI            string `json:"ui"`
	Version       string `json:"version"`
}

// probeReady reads /api/ready once. A connection failure is an error, never a
// fabricated zero value.
func probeReady(base string) (readyReport, error) {
	var r readyReport
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimRight(base, "/") + "/api/ready")
	if err != nil {
		return r, fmt.Errorf("cannot reach %s: %w", base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return r, fmt.Errorf("/api/ready answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return r, fmt.Errorf("/api/ready returned unreadable JSON: %w", err)
	}
	return r, nil
}

// waitReady polls until /api/ready answers status=ready or the budget expires.
func waitReady(base string, attempts int, gap time.Duration) (readyReport, error) {
	var last readyReport
	var lastErr error
	for i := 0; i < attempts; i++ {
		last, lastErr = probeReady(base)
		if lastErr == nil && last.Status == "ready" {
			return last, nil
		}
		time.Sleep(gap)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("/api/ready never reported status=ready")
	}
	return last, lastErr
}

// serviceActive reports whether systemd considers the unit running. It is
// informational: readiness is decided by the HTTP probe above.
func serviceActive(r runner, unit string) bool {
	if !have("systemctl") {
		return false
	}
	_, err := r.run("systemctl", "is-active", "--quiet", unit)
	return err == nil
}

// requireInstalled fails a command early when the host is not an installation.
// Saying "not installed" is far better than a half-run mutation.
func requireInstalled(p paths) error {
	if !fileExists(p.binary()) {
		return fmt.Errorf("veylixd is not installed at %s - run install.sh first", p.binary())
	}
	return nil
}

// requireVersionAgreement compares two probes. It returns an error when the new
// schema version went backwards, which means the binary is older than the
// database and the upgrade must not be considered successful.
func requireVersionAgreement(before, after readyReport) error {
	if after.SchemaVersion < before.SchemaVersion {
		return fmt.Errorf("schema went backwards: %d -> %d; restore the pre-upgrade backup",
			before.SchemaVersion, after.SchemaVersion)
	}
	return nil
}

// takeBackupBefore is the single destructive-safety step shared by `update` and
// `rollback`: run the shipped backup script, and refuse to continue when it is
// missing or fails.
//
// It returns the backup DIRECTORY the script reported, when the script names
// one. That value is what lets a later step restore exactly the state that was
// just replaced, instead of telling an operator to go find a timestamp.
//
// `rollback` used to print "rollback complete. If the database was migrated,
// also restore the pre-upgrade backup" having taken no backup at all - a
// recovery instruction pointing at a file that did not exist.
func takeBackupBefore(r runner, stepName string, dryRun bool) (dir string, err error) {
	script := backupScript()
	if !fileExists(script) {
		return "", fmt.Errorf("refusing to %s without a backup: %s not found "+
			"(it ships in the release bundle as deploy/veylix-backup.sh; "+
			"set VEYLIX_DEPLOY_DIR to the directory that holds it)", stepName, script)
	}

	step("backup")
	if dryRun {
		out("[dry-run] bash %s backup", script)
		return "", nil
	}

	text, runErr := r.run("bash", script, "backup")
	if text != "" {
		out("%s", strings.TrimSpace(redact(text)))
	}
	if runErr != nil {
		return "", fmt.Errorf("backup failed; %s was not started: %w", stepName, runErr)
	}
	return backupDirFromOutput(text, r.backupRoot), nil
}

// backupDirFromOutput finds the directory a backup reported. The shipped script
// prints its destination; an absent match is not an error, because the caller
// can still describe the default root.
//
// The match is rooted at the backup root the CLI is configured with
// (p.BackupDir) rather than "any absolute path", so a line of prose cannot be
// mistaken for a directory on POSIX while a Windows-style path still matches on
// Windows. The path must also exist, so a stale message is never trusted.
func backupDirFromOutput(text, backupRoot string) string {
	root := strings.TrimRight(backupRoot, `/\`)
	if root == "" {
		return ""
	}
	for _, line := range strings.Split(text, "\n") {
		for _, field := range strings.Fields(line) {
			f := strings.TrimRight(field, ",.;:")
			if !strings.HasPrefix(f, root) {
				continue
			}
			if fileExists(f) {
				return f
			}
		}
	}
	return ""
}

// statusDoc is the machine-readable form of `status --json`. It is a named type
// rather than an anonymous struct so every consumer (including tests) sees the
// same shape.
type statusDoc struct {
	Installed     bool   `json:"installed"`
	Binary        string `json:"binary"`
	ServiceActive bool   `json:"service_active"`
	Reachable     bool   `json:"reachable"`
	Ready         bool   `json:"ready"`
	SchemaVersion int    `json:"schema_version"`
	UI            string `json:"ui"`
	DataDirSizeMB int64  `json:"data_dir_size_mb"`
	Error         string `json:"error,omitempty"`
}

// readinessWord turns the three probe outcomes into one honest word. There is no
// fourth state: "probably fine" is not one of the options.
func (d statusDoc) readinessWord() string {
	switch {
	case d.Ready:
		return "ready"
	case d.Reachable:
		return "reachable but not ready"
	default:
		return "unreachable"
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printJSONErr writes the document, then returns an error carrying message, so
// the caller exits non-zero while the machine-readable body is still complete.
func printJSONErr(v any, message string) error {
	if err := printJSON(v); err != nil {
		return err
	}
	return fmt.Errorf("%s", message)
}

func activeWord(active bool) string {
	if active {
		return "active"
	}
	return "inactive"
}

// mkdirAll creates a directory tree through the runner so --dry-run covers it.
func (r runner) mkdirAll(dir string) error {
	if r.dryRun {
		out("[dry-run] mkdir -p %s", dir)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	return nil
}

// resolveHost returns the first IPv4 address for host, or "" when it does not
// resolve. An empty answer is the honest one: it means "no A record found", not
// "probably fine".
func resolveHost(host string) string {
	addrs, err := net.LookupIP(host)
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if v4 := a.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}
