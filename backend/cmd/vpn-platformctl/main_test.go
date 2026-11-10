package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// newTree builds an isolated installation tree under t.TempDir(). Nothing here
// reads or writes a real data directory, real keys or a live database.
func newTree(t *testing.T) paths {
	t.Helper()
	root := t.TempDir()
	p := paths{
		BinDir:    filepath.Join(root, "bin"),
		ConfigDir: filepath.Join(root, "etc"),
		DataDir:   filepath.Join(root, "lib"),
		BackupDir: filepath.Join(root, "backups"),
		UnitPath:  filepath.Join(root, "etc", "veylixd.service"),
		Caddyfile: filepath.Join(root, "etc", "Caddyfile"),
		LockPath:  filepath.Join(root, "run", "install.lock"),
	}
	for _, d := range []string{p.BinDir, p.ConfigDir, p.DataDir, p.BackupDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return p
}

// write puts a file in the tree with the given content.
func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// fakeReady serves a minimal /api/ready so a probe can be exercised without a
// real server.
func fakeReady(t *testing.T, schema int, ui string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ready" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(readyReport{
			Status:        "ready",
			SchemaVersion: schema,
			UI:            ui,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeReadyVersion is fakeReady with an explicit build version, so a downgrade
// decision (which compares versions) can be exercised without a real release.
func fakeReadyVersion(t *testing.T, schema int, ui, ver string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ready" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(readyReport{
			Status:        "ready",
			SchemaVersion: schema,
			UI:            ui,
			Version:       ver,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------------------
// redact / fingerprint
// ---------------------------------------------------------------------------

func TestRedactStripsEverySecretShape(t *testing.T) {
	key := strings.Repeat("A", 43) + "="
	cases := []struct{ in, mustNotContain string }{
		{"VEYLIX_ENCRYPTION_KEY=" + key, key},
		{"Authorization: Bearer abc.def.ghi", "abc.def.ghi"},
		{"secret GOCSPX-abcdefghijklmnop", "GOCSPX-abcdefghijklmnop"},
		{"value " + key, key},
	}
	for _, c := range cases {
		got := redact(c.in)
		if strings.Contains(got, c.mustNotContain) {
			t.Errorf("redact(%q) = %q, still contains the secret", c.in, got)
		}
	}
}

func TestRedactLeavesOrdinaryTextAlone(t *testing.T) {
	for _, s := range []string{
		"/var/lib/veylix/veylix.db",
		"vpn.example.com",
		"systemctl restart veylixd",
		"sha256 sums",
	} {
		if got := redact(s); got != s {
			t.Errorf("redact(%q) = %q, want it unchanged", s, got)
		}
	}
}

func TestFingerprintNeverRevealsTheValue(t *testing.T) {
	p := newTree(t)
	keyPath := filepath.Join(p.ConfigDir, "enc.key")
	write(t, keyPath, "super-secret-key-material")

	fp := fingerprint(keyPath)
	if strings.Contains(fp, "super-secret") {
		t.Fatalf("fingerprint leaked the key: %q", fp)
	}
	if len(fp) != 8 {
		t.Fatalf("fingerprint = %q, want 8 hex chars", fp)
	}
	if got := fingerprint(filepath.Join(p.ConfigDir, "absent.key")); got != "absent" {
		t.Fatalf("fingerprint of a missing file = %q, want \"absent\"", got)
	}
}

// ---------------------------------------------------------------------------
// status: not-installed must fail without creating state
// ---------------------------------------------------------------------------

func TestStatusReportsNotInstalledWithoutWritingAnything(t *testing.T) {
	p := newTree(t)
	err := cmdStatus(p, []string{"--base", "http://127.0.0.1:1"})
	if err == nil {
		t.Fatal("status on an empty tree must fail, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error = %v, want it to say not installed", err)
	}
	if fileExists(p.binary()) || fileExists(p.dbFile()) {
		t.Fatal("status created installation state it should not have")
	}
}

func TestStatusJSONCarriesEveryField(t *testing.T) {
	p := newTree(t)
	doc := statusDoc{Installed: false, Binary: p.binary(), Error: "not installed"}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"installed", "binary", "service_active", "reachable",
		"ready", "schema_version", "ui", "data_dir_size_mb", "error"} {
		if _, ok := back[key]; !ok {
			t.Errorf("status JSON is missing %q", key)
		}
	}
}

func TestReadinessWordHasNoFourthState(t *testing.T) {
	cases := []struct {
		doc  statusDoc
		want string
	}{
		{statusDoc{Ready: true, Reachable: true}, "ready"},
		{statusDoc{Reachable: true}, "reachable but not ready"},
		{statusDoc{}, "unreachable"},
	}
	for _, c := range cases {
		if got := c.doc.readinessWord(); got != c.want {
			t.Errorf("readinessWord = %q, want %q", got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// dry-run must not mutate
// ---------------------------------------------------------------------------

func TestDryRunUninstallDeletesNothing(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "binary")
	write(t, p.dbFile(), "database")
	write(t, p.UnitPath, "unit")

	if err := cmdUninstall(p, []string{"--dry-run", "--base", "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("dry-run uninstall: %v", err)
	}
	for _, f := range []string{p.binary(), p.dbFile(), p.UnitPath} {
		if !fileExists(f) {
			t.Fatalf("dry-run uninstall removed %s", f)
		}
	}
}

func TestDryRunPurgeDeletesNothing(t *testing.T) {
	p := newTree(t)
	write(t, p.envFile(), "VEYLIX_PUBLIC_BASE_URL=https://vpn.example.com\n")
	write(t, p.dbFile(), "database")

	if err := cmdUninstall(p, []string{"--dry-run", "--purge-data"}); err != nil {
		t.Fatalf("dry-run purge: %v", err)
	}
	if !fileExists(p.dbFile()) {
		t.Fatal("dry-run purge deleted the database")
	}
}

// ---------------------------------------------------------------------------
// uninstall: data preserved by default, purge double-gated
// ---------------------------------------------------------------------------

func TestUninstallPreservesDataByDefault(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "binary")
	write(t, p.prevBin(), "previous")
	write(t, p.UnitPath, "unit")
	write(t, p.dbFile(), "database")
	write(t, p.encKey(), "enc")

	if err := cmdUninstall(p, []string{"--base", "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	// Binaries and the unit are gone...
	for _, f := range []string{p.binary(), p.prevBin(), p.UnitPath} {
		if fileExists(f) {
			t.Errorf("%s should have been removed", f)
		}
	}
	// ...but the database and the keys are not. This is the invariant.
	for _, f := range []string{p.dbFile(), p.encKey()} {
		if !fileExists(f) {
			t.Errorf("%s must be preserved by a default uninstall", f)
		}
	}
}

func TestPurgeRefusesWithoutAConfiguredDomain(t *testing.T) {
	p := newTree(t)
	write(t, p.dbFile(), "database")
	// No env file, so no domain to type: the purge must refuse, not proceed.
	err := cmdUninstall(p, []string{"--purge-data"})
	if err == nil {
		t.Fatal("purge without a configured domain must refuse")
	}
	if !strings.Contains(err.Error(), "cannot purge") {
		t.Fatalf("error = %v, want a cannot-purge refusal", err)
	}
	if !fileExists(p.dbFile()) {
		t.Fatal("the refusal still deleted the database")
	}
}

// ---------------------------------------------------------------------------
// checksum enforcement
// ---------------------------------------------------------------------------

func TestVerifyChecksumAcceptsAndRejects(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "veylixd-linux-amd64")
	sums := filepath.Join(dir, "SHA256SUMS")
	write(t, artifact, "artifact payload")

	// A deliberately wrong digest must be rejected AND the artifact deleted.
	wrong := strings.Repeat("0", 64)
	write(t, sums, wrong+"  veylixd-linux-amd64\n")
	if err := verifyChecksum(artifact, sums, "veylixd-linux-amd64"); err == nil {
		t.Fatal("a wrong digest must be rejected")
	}
	if fileExists(artifact) {
		t.Fatal("a mismatched artifact must be deleted")
	}

	// Now with the real digest.
	write(t, artifact, "artifact payload")
	real := sha256HexOf(t, artifact)
	write(t, sums, real+"  veylixd-linux-amd64\n")
	if err := verifyChecksum(artifact, sums, "veylixd-linux-amd64"); err != nil {
		t.Fatalf("a matching digest must be accepted: %v", err)
	}
}

func TestVerifyChecksumAbortsOnMissingEntry(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "veylixd-linux-arm64")
	sums := filepath.Join(dir, "SHA256SUMS")
	write(t, artifact, "payload")
	write(t, sums, strings.Repeat("a", 64)+"  veylixd-linux-amd64\n")

	err := verifyChecksum(artifact, sums, "veylixd-linux-arm64")
	if err == nil || !strings.Contains(err.Error(), "no entry") {
		t.Fatalf("error = %v, want a missing-entry refusal", err)
	}
}

// sha256HexOf is the test's own digest helper. It is deliberately independent of
// the production code path so a bug in verifyChecksum cannot make the test agree
// with itself.
func sha256HexOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// domain validation and rendering
// ---------------------------------------------------------------------------

func TestValidateDomain(t *testing.T) {
	good := []string{"vpn.example.com", "a.b.co.uk", "sub-domain.example.org"}
	for _, d := range good {
		if err := validateDomain(d); err != nil {
			t.Errorf("validateDomain(%q) = %v, want nil", d, err)
		}
	}
	bad := []string{"", "localhost", "1.2.3.4", "notadomain", ".example.com",
		"example.com.", "a..b.com"}
	for _, d := range bad {
		if err := validateDomain(d); err == nil {
			t.Errorf("validateDomain(%q) = nil, want a refusal", d)
		}
	}
}

func TestRenderDomainReplacesThePlaceholder(t *testing.T) {
	template := "vpn.example.com {\n  reverse_proxy 127.0.0.1:8080\n}\n"
	got := renderDomain(template, "vpn.acme.net")
	if !strings.Contains(got, "vpn.acme.net {") {
		t.Fatalf("rendered config = %q, want the new domain", got)
	}
	if strings.Contains(got, "vpn.example.com") {
		t.Fatalf("rendered config still holds the placeholder: %q", got)
	}
}

// ---------------------------------------------------------------------------
// repair: refuses when not installed, never touches the database
// ---------------------------------------------------------------------------

func TestRepairRefusesWhenNotInstalled(t *testing.T) {
	p := newTree(t)
	if err := cmdRepair(p, []string{"--dry-run"}); err == nil {
		t.Fatal("repair on an empty tree must refuse")
	}
}

func TestRepairDryRunTouchesNothing(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "binary")
	write(t, p.envFile(), "VEYLIX_ADDR=127.0.0.1:8080\n")
	write(t, p.dbFile(), "database")

	if err := cmdRepair(p, []string{"--dry-run", "--base", "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("dry-run repair: %v", err)
	}
	data, err := os.ReadFile(p.dbFile())
	if err != nil {
		t.Fatalf("database disappeared: %v", err)
	}
	if string(data) != "database" {
		t.Fatal("repair modified the database; it must never do that")
	}
}

// ---------------------------------------------------------------------------
// rollback preconditions
// ---------------------------------------------------------------------------

func TestRollbackWithoutPreviousBinaryRefuses(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	err := cmdRollback(p, []string{"--base", "http://127.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "no previous binary") {
		t.Fatalf("error = %v, want a no-previous-binary refusal", err)
	}
}

// ---------------------------------------------------------------------------
// rollback takes its own backup
//
// The defect this pins: cmdRollback used to swap the binary and then print
// "also restore the pre-upgrade backup", having taken no backup at all. Every
// test below is a gate on the recovery path actually existing.
// ---------------------------------------------------------------------------

// fakeBackupScript writes a stand-in for deploy/veylix-backup.sh that reports a
// directory, so the parser and the ordering can be tested with no real SQLite
// and no real data directory.
func fakeBackupScript(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "veylix-backup.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write fake backup script: %v", err)
	}
	t.Setenv("VEYLIX_DEPLOY_DIR", dir)
	return script
}

func TestRollbackRefusesWithoutBackupScript(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	write(t, p.prevBin(), "previous")
	t.Setenv("VEYLIX_DEPLOY_DIR", filepath.Join(t.TempDir(), "no-such-dir"))

	err := cmdRollback(p, []string{"--base", "http://127.0.0.1:1"})
	if err == nil {
		t.Fatal("rollback proceeded with no backup script; it must refuse")
	}
	if !strings.Contains(err.Error(), "without a backup") {
		t.Fatalf("error = %v, want a refuse-without-a-backup error", err)
	}
	// The refusal must happen BEFORE the swap, or the point is lost.
	if current, _ := os.ReadFile(p.binary()); string(current) != "current" {
		t.Fatalf("the binary was replaced before the backup guard: %q", current)
	}
}

func TestRollbackDryRunPrintsTheBackupAndChangesNothing(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	write(t, p.prevBin(), "previous")
	fakeBackupScript(t, `exit 0`)

	if err := cmdRollback(p, []string{"--dry-run", "--base", "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("dry-run rollback returned %v, want nil", err)
	}
	if current, _ := os.ReadFile(p.binary()); string(current) != "current" {
		t.Fatalf("dry-run replaced the binary: %q", current)
	}
}

func TestRollbackBackupFailureStopsBeforeTheSwap(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	write(t, p.prevBin(), "previous")
	fakeBackupScript(t, `echo "veylix-backup: disk full" >&2; exit 1`)

	err := cmdRollback(p, []string{"--base", "http://127.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "backup failed") {
		t.Fatalf("error = %v, want a backup-failed error", err)
	}
	if current, _ := os.ReadFile(p.binary()); string(current) != "current" {
		t.Fatalf("the binary was replaced despite a failed backup: %q", current)
	}
}

func TestRollbackRestoreDefaultsToTheBackupItJustTook(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	write(t, p.prevBin(), "previous")

	// The backup root the CLI is configured with, exactly as newTree() lays it out.
	backupDir := filepath.Join(p.BackupDir, "2026-11-08_1200")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backup: %v", err)
	}
	fakeBackupScript(t, `echo "veylix-backup: backup complete under `+backupDir+`"`)

	// The parser must find the directory the backup just reported...
	if got := backupDirFromOutput("veylix-backup: backup complete under "+backupDir, p.BackupDir); got != backupDir {
		t.Fatalf("backupDirFromOutput = %q, want %q", got, backupDir)
	}
	// ...and the dry run must complete without touching anything.
	if err := cmdRollback(p, []string{"--dry-run", "--base", "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("dry-run rollback returned %v, want nil", err)
	}
	if current, _ := os.ReadFile(p.binary()); string(current) != "current" {
		t.Fatalf("dry-run replaced the binary: %q", current)
	}
}

func TestRollbackSkipBackupIsDeliberateAndAllowed(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	write(t, p.prevBin(), "previous")
	// No backup script at all: only the explicit flag may get past the guard.
	t.Setenv("VEYLIX_DEPLOY_DIR", filepath.Join(t.TempDir(), "no-such-dir"))

	if err := cmdRollback(p, []string{"--dry-run", "--skip-backup", "--base", "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("--skip-backup must let the dry run proceed, got %v", err)
	}
}

func TestBackupDirFromOutputOnlyTrustsTheConfiguredRoot(t *testing.T) {
	p := newTree(t)

	if got := backupDirFromOutput("", p.BackupDir); got != "" {
		t.Errorf("empty output must yield no directory, got %q", got)
	}
	// Prose naming a path outside the backup root must not be trusted, even if
	// that path exists: it is not something this host's backup wrote.
	if got := backupDirFromOutput("see "+t.TempDir(), p.BackupDir); got != "" {
		t.Errorf("an out-of-root path must not be returned, got %q", got)
	}
	// A missing directory inside the root is equally untrusted.
	if got := backupDirFromOutput(filepath.Join(p.BackupDir, "gone"), p.BackupDir); got != "" {
		t.Errorf("a non-existent path must not be returned, got %q", got)
	}
	// Trailing punctuation from prose is stripped.
	real := filepath.Join(p.BackupDir, "2026-11-08")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := backupDirFromOutput("backup complete under "+real+".", p.BackupDir); got != real {
		t.Errorf("backupDirFromOutput = %q, want %q", got, real)
	}
}

// ---------------------------------------------------------------------------
// schema agreement
// ---------------------------------------------------------------------------

func TestRequireVersionAgreement(t *testing.T) {
	before := readyReport{SchemaVersion: 21}
	if err := requireVersionAgreement(before, readyReport{SchemaVersion: 21}); err != nil {
		t.Errorf("equal schema must pass: %v", err)
	}
	if err := requireVersionAgreement(before, readyReport{SchemaVersion: 22}); err != nil {
		t.Errorf("a forward migration must pass: %v", err)
	}
	if err := requireVersionAgreement(before, readyReport{SchemaVersion: 20}); err == nil {
		t.Error("a backwards schema must fail")
	}
}

// ---------------------------------------------------------------------------
// update preconditions
// ---------------------------------------------------------------------------

func TestUpdateRefusesWithoutBackupScript(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "binary")
	t.Setenv("VEYLIX_DEPLOY_DIR", filepath.Join(t.TempDir(), "no-such-dir"))

	err := cmdUpdate(p, []string{"--dry-run", "--base", "http://127.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "without a backup") {
		t.Fatalf("error = %v, want a refuse-without-backup error", err)
	}
}

func TestUpdateRejectsUnknownChannel(t *testing.T) {
	p := newTree(t)
	err := cmdUpdate(p, []string{"--channel", "nightly"})
	if err == nil || !strings.Contains(err.Error(), "unknown channel") {
		t.Fatalf("error = %v, want an unknown-channel usage error", err)
	}
}

func TestUpdateRefusesWhenNotInstalled(t *testing.T) {
	p := newTree(t)
	err := cmdUpdate(p, []string{"--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error = %v, want a not-installed refusal", err)
	}
}

// ---------------------------------------------------------------------------
// command dispatch
// ---------------------------------------------------------------------------

func TestUnknownCommandIsAUsageError(t *testing.T) {
	err := run([]string{"frobnicate"})
	if err == nil {
		t.Fatal("an unknown command must not be a silent success")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, want an unknown-command error", err)
	}
}

func TestKnownCommandsDoNotPanic(t *testing.T) {
	// Every command is reachable and returns rather than panicking on empty args.
	for _, cmd := range []string{"status", "doctor", "backup", "repair", "update",
		"rollback", "domain-set", "restore", "uninstall"} {
		_ = run([]string{cmd, "--dry-run", "--base", "http://127.0.0.1:1"})
	}
}

func TestReleaseURLsMatchTheArtifactNames(t *testing.T) {
	if got := artifactName("amd64"); got != "veylixd-linux-amd64" {
		t.Errorf("artifactName = %q", got)
	}
	if got := cliArtifactName("arm64"); got != "vpn-platformctl-linux-arm64" {
		t.Errorf("cliArtifactName = %q", got)
	}
	if got := releaseBaseURL("o/r", ""); got != "https://github.com/o/r/releases/latest/download" {
		t.Errorf("latest release base = %q", got)
	}
	if got := releaseBaseURL("o/r", "v1.2.3"); got != "https://github.com/o/r/releases/download/v1.2.3" {
		t.Errorf("pinned release base = %q", got)
	}
}

// ---------------------------------------------------------------------------
// configuredDomain parses the env file without touching a network
// ---------------------------------------------------------------------------

func TestConfiguredDomain(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "veylix.env")

	write(t, env, "VEYLIX_ADDR=127.0.0.1:8080\nVEYLIX_PUBLIC_BASE_URL=https://vpn.acme.net\n")
	if got := configuredDomain(env); got != "vpn.acme.net" {
		t.Fatalf("configuredDomain = %q, want vpn.acme.net", got)
	}

	write(t, env, "VEYLIX_ADDR=127.0.0.1:8080\n")
	if got := configuredDomain(env); got != "" {
		t.Fatalf("configuredDomain = %q, want empty when unset", got)
	}

	if got := configuredDomain(filepath.Join(dir, "absent.env")); got != "" {
		t.Fatalf("configuredDomain = %q, want empty for a missing file", got)
	}
}

// ---------------------------------------------------------------------------
// appendEnvIfMissing never rewrites an existing choice
// ---------------------------------------------------------------------------

func TestAppendEnvIfMissingDoesNotRewrite(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "veylix.env")
	write(t, env, "VEYLIX_PUBLIC_BASE_URL=https://old.example.com\n")

	if err := appendEnvIfMissing(env, "VEYLIX_PUBLIC_BASE_URL",
		"VEYLIX_PUBLIC_BASE_URL=https://new.example.com\n"); err != nil {
		t.Fatalf("appendEnvIfMissing: %v", err)
	}
	data, _ := os.ReadFile(env)
	if !strings.Contains(string(data), "old.example.com") {
		t.Fatal("appendEnvIfMissing overwrote an existing value")
	}
	if strings.Contains(string(data), "new.example.com") {
		t.Fatal("appendEnvIfMissing appended a second value for the same key")
	}
}

// ---------------------------------------------------------------------------
// readiness probing against a fake server
// ---------------------------------------------------------------------------

func TestProbeReadyReadsTheRealFields(t *testing.T) {
	srv := fakeReady(t, 21, "embedded:81_files")
	rep, err := probeReady(srv.URL)
	if err != nil {
		t.Fatalf("probeReady: %v", err)
	}
	if rep.SchemaVersion != 21 || rep.UI != "embedded:81_files" {
		t.Fatalf("probeReady = %+v, want schema 21 and the ui text", rep)
	}
}

func TestProbeReadyFailsOnAnUnreachableService(t *testing.T) {
	if _, err := probeReady("http://127.0.0.1:1"); err == nil {
		t.Fatal("an unreachable service must be an error, not a zero value")
	}
}

func TestProbeReadyFailsOnANon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"db_unavailable"}`, http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	if _, err := probeReady(srv.URL); err == nil {
		t.Fatal("a 503 must be an error")
	}
}

// ---------------------------------------------------------------------------
// removeAll guards the host
// ---------------------------------------------------------------------------

func TestRemoveAllRefusesSuspiciousPaths(t *testing.T) {
	r := runner{}
	// Empty, the current directory and every root the tool refuses to treat as a
	// leaf are cross-platform. The Windows drive-root spelling is NOT: on Linux
	// `C:\` is an ordinary directory name, so asserting a refusal there tested
	// the runner's OS rather than the guard. It is checked only where it means
	// what it says.
	always := []string{"", ".", "/", `\`, "/var", "/etc", "/usr", "/home", "/root",
		"/tmp", "/opt", "/srv"}
	if runtime.GOOS == "windows" {
		always = append(always, `C:\`, `C:/`, `D:\`)
	}
	for _, dir := range always {
		if err := r.removeAll(dir); err == nil {
			t.Errorf("removeAll(%q) = nil, want a refusal", dir)
		}
	}
}

func TestGuardDeletableAllowsALeaf(t *testing.T) {
	for _, dir := range []string{
		"/var/lib/veylix",
		"/etc/veylix",
		filepath.Join(t.TempDir(), "veylix-data"),
	} {
		if err := guardDeletable(dir); err != nil {
			t.Errorf("guardDeletable(%q) = %v, want nil", dir, err)
		}
	}
}

func TestRemoveAllDeletesALeafDirectory(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "veylix-data")
	write(t, filepath.Join(victim, "veylix.db"), "db")

	r := runner{}
	if err := r.removeAll(victim); err != nil {
		t.Fatalf("removeAll: %v", err)
	}
	if fileExists(victim) {
		t.Fatal("removeAll did not delete the directory")
	}
}

// ---------------------------------------------------------------------------
// update --allow-downgrade / --restore-backup
//
// Migrations are forward-only, so an older binary cannot read a database a
// newer one migrated. These tests pin the two gates that keep a downgrade
// deliberate: the flag to mean it, the restore to be able to do it.
// ---------------------------------------------------------------------------

func TestUpdateDowngradeRefusedWithoutAllowFlag(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	srv := fakeReadyVersion(t, 21, "embedded:85_files", "v1.5.0")

	err := cmdUpdate(p, []string{"--base", srv.URL, "--version", "v1.3.0"})
	var ue usageError
	if !errors.As(err, &ue) {
		t.Fatalf("cmdUpdate = %v, want a usageError", err)
	}
	if !strings.Contains(err.Error(), "refusing to downgrade") {
		t.Fatalf("message = %q, want it to name the downgrade refusal", err.Error())
	}
}

func TestUpdateDowngradeNeedsRestoreBackup(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	srv := fakeReadyVersion(t, 21, "embedded:85_files", "v1.5.0")

	err := cmdUpdate(p, []string{"--base", srv.URL, "--version", "v1.3.0", "--allow-downgrade"})
	var ue usageError
	if !errors.As(err, &ue) {
		t.Fatalf("cmdUpdate = %v, want a usageError", err)
	}
	if !strings.Contains(err.Error(), "--restore-backup") {
		t.Fatalf("message = %q, want it to require --restore-backup", err.Error())
	}
}

func TestUpdateDowngradeRejectsMissingBackupDir(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	srv := fakeReadyVersion(t, 21, "embedded:85_files", "v1.5.0")
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	err := cmdUpdate(p, []string{"--base", srv.URL, "--version", "v1.3.0",
		"--allow-downgrade", "--restore-backup", missing})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("cmdUpdate = %v, want a missing-directory error", err)
	}
}

func TestUpdateUpgradeIsNotTreatedAsDowngrade(t *testing.T) {
	p := newTree(t)
	write(t, p.binary(), "current")
	srv := fakeReadyVersion(t, 21, "embedded:85_files", "v1.3.0")

	// A newer target must pass the downgrade guard and stop later at the
	// missing backup script, never with a downgrade refusal.
	err := cmdUpdate(p, []string{"--base", srv.URL, "--version", "v1.9.0"})
	if err == nil {
		t.Fatal("expected the run to stop at the missing backup script")
	}
	if strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("an upgrade was misread as a downgrade: %v", err)
	}
	if !strings.Contains(err.Error(), "backup") {
		t.Fatalf("message = %q, want it to stop at the backup step", err.Error())
	}
}

// TestVersionsReadsTheReleaseFeed proves the command is wired into the
// dispatcher and reads a feed, offline, via the VEYLIX_RELEASE_API override.
func TestVersionsReadsTheReleaseFeed(t *testing.T) {
	t.Setenv("VEYLIX_REPO", "owner/repo")
	t.Setenv("VEYLIX_BASE_URL", "http://127.0.0.1:1") // unreachable: fall back to the build version

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"tag_name": "v1.5.0", "published_at": "2026-11-10T00:00:00Z", "html_url": "https://x/v1.5.0"},
			{"tag_name": "v1.3.0", "published_at": "2026-10-28T00:00:00Z", "html_url": "https://x/v1.3.0"},
		})
	}))
	t.Cleanup(feed.Close)
	t.Setenv("VEYLIX_RELEASE_API", feed.URL)

	if err := run([]string{"versions", "--json"}); err != nil {
		t.Fatalf("versions returned %v, want nil", err)
	}
}
