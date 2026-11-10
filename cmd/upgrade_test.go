package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"nga/pkg/model"
)

func TestUpgradeCommandRegistered(t *testing.T) {
	cmd, _, err := RootCmd.Find([]string{"upgrade"})
	if err != nil {
		t.Fatalf("upgrade command not registered: %v", err)
	}

	for _, flagName := range []string{"check", "version", "force", "no-checksum", "repo", "api-base", "bin"} {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("missing --%s flag on upgrade", flagName)
		}
	}

	for _, alias := range []string{"update", "self-update"} {
		found := false
		for _, a := range cmd.Aliases {
			if a == alias {
				found = true
			}
		}
		if !found {
			t.Errorf("missing alias %q", alias)
		}
	}
}

func TestUpgradeHelpRenders(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"upgrade", "--help"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("GitHub Release")) {
		t.Errorf("help output missing release source: %s", buf.String())
	}
}

func TestFirstConfigured(t *testing.T) {
	if got := firstConfigured("", "  ", " fimreal/nga-cli "); got != "fimreal/nga-cli" {
		t.Errorf("firstConfigured = %q, want the trimmed value", got)
	}
	if got := firstConfigured("", ""); got != "" {
		t.Errorf("firstConfigured = %q, want empty", got)
	}
}

func resetUpgradeFlags() {
	reset := func(fs *pflag.FlagSet) {
		if fs != nil {
			fs.VisitAll(func(f *pflag.Flag) {
				_ = f.Value.Set(f.DefValue)
				f.Changed = false
			})
		}
	}
	reset(RootCmd.PersistentFlags())
	reset(RootCmd.Flags())
	if cmd, _, err := RootCmd.Find([]string{"upgrade"}); err == nil {
		reset(cmd.Flags())
	}
	upgradeCheck = false
	upgradeTag = ""
	upgradeForce = false
	upgradeNoChecksum = false
	upgradeRepo = ""
	upgradeAPIBase = ""
	upgradeBin = ""
	flagJSON = false
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = oldStdout
	return <-outC
}

func createTarGzBinary(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "./" + name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func setupMockUpgradeServer(t *testing.T, tag, binaryName string, binaryContent []byte, badChecksum bool) (*httptest.Server, string) {
	t.Helper()
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goos == "linux" && goarch == "arm" {
		goarch = "armv7"
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	assetName := fmt.Sprintf("nga_%s_%s-%s%s", tag, goos, goarch, ext)
	archive := createTarGzBinary(t, binaryName, binaryContent)

	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	checksumLine := fmt.Sprintf("%s  %s\n", digest, assetName)
	if badChecksum {
		checksumLine = fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), assetName)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/fimreal/nga-cli/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"tag_name": %q,
			"name": "NGA CLI Release %s",
			"html_url": "https://github.com/fimreal/nga-cli/releases/tag/%s",
			"published_at": "2026-11-01T00:00:00Z",
			"assets": [
				{"name": %q, "size": %d, "browser_download_url": "http://%s/files/%s"},
				{"name": "checksums.txt", "size": 128, "browser_download_url": "http://%s/files/checksums.txt"}
			]
		}`, tag, tag, tag, assetName, len(archive), r.Host, assetName, r.Host)
	})
	mux.HandleFunc("/files/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/files/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, checksumLine)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, assetName
}

func TestUpgradeCmdCheckOnlyKeepsBinary(t *testing.T) {
	srv, _ := setupMockUpgradeServer(t, "v9.9.9", "nga", []byte("new-binary-bytes"), false)
	dir := t.TempDir()
	target := filepath.Join(dir, "nga")
	if err := os.WriteFile(target, []byte("old-binary-content"), 0755); err != nil {
		t.Fatal(err)
	}

	resetUpgradeFlags()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"upgrade", "--api-base", srv.URL, "--bin", target, "--check"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error running upgrade --check: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old-binary-content" {
		t.Errorf("binary content must stay untouched with --check, got %q", got)
	}
}

func TestUpgradeCmdChecksumMismatchKeepsBinary(t *testing.T) {
	srv, _ := setupMockUpgradeServer(t, "v9.9.9", "nga", []byte("new-binary-bytes"), true)
	dir := t.TempDir()
	target := filepath.Join(dir, "nga")
	if err := os.WriteFile(target, []byte("old-binary-content"), 0755); err != nil {
		t.Fatal(err)
	}

	resetUpgradeFlags()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"upgrade", "--api-base", srv.URL, "--bin", target, "--force"})

	err := RootCmd.Execute()
	if err == nil {
		t.Fatal("expected SHA256 checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "SHA256") {
		t.Errorf("unexpected error message: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old-binary-content" {
		t.Errorf("binary content must stay untouched on checksum mismatch, got %q", got)
	}
}

func TestUpgradeCmdJSONOutput(t *testing.T) {
	srv, _ := setupMockUpgradeServer(t, "v9.9.9", "nga", []byte("new-binary-bytes"), false)
	dir := t.TempDir()
	target := filepath.Join(dir, "nga")
	if err := os.WriteFile(target, []byte("old-binary-content"), 0755); err != nil {
		t.Fatal(err)
	}

	resetUpgradeFlags()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"upgrade", "--api-base", srv.URL, "--bin", target, "--check", "--json"})

	out := captureStdout(t, func() {
		if err := RootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error running upgrade --json: %v", err)
		}
	})

	var res model.UpgradeResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %v, raw output: %q", err, out)
	}
	if res.TargetVersion != "v9.9.9" {
		t.Errorf("target_version = %q, want v9.9.9", res.TargetVersion)
	}
	if !res.UpdateAvailable {
		t.Errorf("update_available should be true")
	}
	if res.Applied {
		t.Errorf("applied should be false for check only")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old-binary-content" {
		t.Errorf("binary content must stay untouched with --check, got %q", got)
	}
}

func TestUpgradeCmdSuccessfulUpgrade(t *testing.T) {
	newBinary := []byte("new-installed-binary")
	srv, _ := setupMockUpgradeServer(t, "v9.9.9", "nga", newBinary, false)
	dir := t.TempDir()
	target := filepath.Join(dir, "nga")
	if err := os.WriteFile(target, []byte("old-binary-content"), 0755); err != nil {
		t.Fatal(err)
	}

	resetUpgradeFlags()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"upgrade", "--api-base", srv.URL, "--bin", target, "--force"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newBinary) {
		t.Errorf("binary was not updated: got %q, want %q", got, newBinary)
	}
}

func TestUpgradeCmdPermissionDeniedKeepsBinary(t *testing.T) {
	srv, _ := setupMockUpgradeServer(t, "v9.9.9", "nga", []byte("new-binary-bytes"), false)
	dir := t.TempDir()
	target := filepath.Join(dir, "nga")
	if err := os.WriteFile(target, []byte("old-binary-content"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755)

	probe, err := os.MkdirTemp(dir, "probe-")
	if err == nil {
		_ = os.Remove(probe)
		_ = os.Chmod(dir, 0755)

		cmd := exec.Command("chattr", "+i", dir)
		if err := cmd.Run(); err != nil {
			t.Skip("skipping permission denied test: running as root and chattr +i is unavailable")
		}
		defer exec.Command("chattr", "-i", dir).Run()
	}

	resetUpgradeFlags()
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"upgrade", "--api-base", srv.URL, "--bin", target, "--force"})

	err = RootCmd.Execute()
	if err == nil {
		t.Fatal("expected permission denied error, got nil")
	}
	if !strings.Contains(err.Error(), "不可写") && !strings.Contains(err.Error(), "创建临时目录失败") {
		t.Errorf("expected permission or unwritable error, got: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old-binary-content" {
		t.Errorf("binary content must stay untouched on permission denied, got %q", got)
	}
}
