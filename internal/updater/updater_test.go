package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const installedBinary = "brand-new-nga-binary"

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "v0.2.0", -1},
		{"0.2.0", "v0.2.0", 0},
		{"v0.2.0", "v0.2.0", 0},
		{"v0.10.0", "v0.9.0", 1},
		{"v0.2", "v0.2.0", 0},
		{"v1.0.0-rc1", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-rc1", 1},
		{"dev", "v0.2.0", -1},
		{"", "v0.2.0", -1},
		{"v0.2.0", "dev", 1},
	}

	for _, c := range cases {
		if got := compareVersion(c.a, c.b); got != c.want {
			t.Errorf("compareVersion(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersionUnknownForDevBuilds(t *testing.T) {
	for _, s := range []string{"dev", "", "main", "nightly"} {
		if !parseVersion(s).unknown {
			t.Errorf("parseVersion(%q) should be unknown", s)
		}
	}
	// Semver build metadata is ignored by precedence, so these stay comparable.
	for _, s := range []string{"v0.2.0", "0.2.0", "v0.2.0+dirty"} {
		if parseVersion(s).unknown {
			t.Errorf("parseVersion(%q) should be known", s)
		}
	}
	if compareVersion("v0.2.0+dirty", "v0.2.0") != 0 {
		t.Errorf("+dirty build metadata should not change precedence")
	}
}

func TestParseChecksums(t *testing.T) {
	valid := strings.Repeat("a", 64)
	data := []byte(valid + "  nga_v0.2.0_linux-amd64.tar.gz\n" + strings.Repeat("b", 64) +
		" *nga_v0.2.0_windows-amd64.zip\nnot-a-digest  broken.txt\n\n")

	sums := parseChecksums(data)
	if len(sums) != 2 {
		t.Fatalf("expected 2 digests, got %d: %v", len(sums), sums)
	}
	if sums["nga_v0.2.0_linux-amd64.tar.gz"] != valid {
		t.Errorf("unexpected digest map: %v", sums)
	}
	if _, ok := sums["broken.txt"]; ok {
		t.Errorf("malformed line should have been skipped")
	}
}

func TestAssetNamePerPlatform(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "nga_v0.2.0_linux-amd64.tar.gz"},
		{"linux", "arm", "nga_v0.2.0_linux-armv7.tar.gz"},
		{"darwin", "arm64", "nga_v0.2.0_darwin-arm64.tar.gz"},
		{"windows", "amd64", "nga_v0.2.0_windows-amd64.zip"},
	}

	for _, c := range cases {
		u := New(Config{GOOS: c.goos, GOARCH: c.goarch})
		if got := u.AssetName("v0.2.0"); got != c.want {
			t.Errorf("%s/%s asset = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestReleaseJSONParsing(t *testing.T) {
	var rel Release
	raw := `{"tag_name":"v0.2.0","name":"NGA CLI Release v0.2.0",
		"html_url":"https://github.com/fimreal/nga-cli/releases/tag/v0.2.0",
		"published_at":"2026-11-01T00:00:00Z","prerelease":false,"draft":false,
		"assets":[{"name":"checksums.txt","size":672,"browser_download_url":"https://github.com/fimreal/nga-cli/releases/download/v0.2.0/checksums.txt"}]}`
	if err := json.Unmarshal([]byte(raw), &rel); err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v0.2.0" || len(rel.Assets) != 1 {
		t.Fatalf("unexpected release: %+v", rel)
	}
	if rel.PublishedAt.IsZero() {
		t.Errorf("published_at should have parsed")
	}
}

// --- end-to-end upgrade tests -------------------------------------------

type fixtureOpts struct {
	tag             string
	platform        string // e.g. "linux-amd64.tar.gz", "windows-amd64.zip"
	entryName       string
	badChecksum     bool
	noChecksumAsset bool
}

// releaseFixture serves a GitHub-shaped release channel: one platform archive
// plus the checksums.txt index the project's release workflow publishes.
type releaseFixture struct {
	srv       *httptest.Server
	assetName string
	digest    string
	archive   []byte
}

func newReleaseFixture(t *testing.T, o fixtureOpts) *releaseFixture {
	t.Helper()

	fx := &releaseFixture{assetName: "nga_" + o.tag + "_" + o.platform}
	if strings.HasSuffix(o.platform, ".zip") {
		fx.archive = zipWithBinary(t, o.entryName, []byte(installedBinary))
	} else {
		fx.archive = tarGzWithBinary(t, o.entryName, []byte(installedBinary))
	}

	fx.digest = digestOf(fx.archive)
	sums := fmt.Sprintf("%s  %s\n", fx.digest, fx.assetName)
	if o.badChecksum {
		sums = fmt.Sprintf("%s  %s\n", strings.Repeat("f", 64), fx.assetName)
	}

	mux := http.NewServeMux()
	releaseHandler := func(w http.ResponseWriter, r *http.Request) {
		// /repos/<repo>/releases/tags/<tag> only knows about the fixture tag.
		if i := strings.Index(r.URL.Path, "/releases/tags/"); i >= 0 {
			if asked := strings.TrimPrefix(r.URL.Path[i+len("/releases/tags/"):], "v"); asked != strings.TrimPrefix(o.tag, "v") {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, releasePayload("http://"+r.Host, o.tag, fx.assetName, o.noChecksumAsset))
	}
	mux.HandleFunc("/repos/fimreal/nga-cli/releases/latest", releaseHandler)
	mux.HandleFunc("/repos/fimreal/nga-cli/releases/tags/", releaseHandler)
	mux.HandleFunc("/files/"+fx.assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fx.archive)
	})
	mux.HandleFunc("/files/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sums)
	})

	fx.srv = httptest.NewServer(mux)
	t.Cleanup(fx.srv.Close)
	return fx
}

func releasePayload(base, tag, assetName string, noChecksum bool) string {
	assets := []string{
		fmt.Sprintf(`{"name": %q, "size": 4096, "browser_download_url": %q}`, assetName, base+"/files/"+assetName),
	}
	if !noChecksum {
		assets = append(assets,
			fmt.Sprintf(`{"name": "checksums.txt", "size": 128, "browser_download_url": %q}`, base+"/files/checksums.txt"))
	}

	return fmt.Sprintf(`{
		"tag_name": %q,
		"name": "NGA CLI Release %s",
		"html_url": "https://github.com/fimreal/nga-cli/releases/tag/%s",
		"published_at": "2026-11-01T00:00:00Z",
		"prerelease": false,
		"draft": false,
		"assets": [%s]
	}`, tag, tag, tag, strings.Join(assets, ","))
}

func tarGzWithBinary(t *testing.T, name string, content []byte) []byte {
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

func zipWithBinary(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// newTestBinary stands in for an installed binary so tests never touch the
// running test executable.
func newTestBinary(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func (fx *releaseFixture) updater(target, goos, goarch string) *Updater {
	return New(Config{
		APIBase:    fx.srv.URL,
		GOOS:       goos,
		GOARCH:     goarch,
		TargetPath: target,
		Client:     fx.srv.Client(),
	})
}

func TestUpgradeApplyReplacesBinary(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "linux-amd64.tar.gz", entryName: "nga"})
	target := newTestBinary(t, "nga")

	res, err := fx.updater(target, "linux", "amd64").
		Upgrade(context.Background(), "v0.1.0", Options{Apply: true, Verify: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.UpdateAvailable || !res.Applied || !res.ChecksumVerified {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.TargetVersion != "v0.9.0" || res.AssetName != fx.assetName || res.Checksum != fx.digest {
		t.Errorf("unexpected result fields: %+v", res)
	}
	if res.AssetSize != int64(len(fx.archive)) {
		t.Errorf("asset_size = %d, want %d", res.AssetSize, len(fx.archive))
	}
	wantTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if res.TargetPath != wantTarget {
		t.Errorf("target_path = %q, want %q", res.TargetPath, wantTarget)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != installedBinary {
		t.Errorf("binary content = %q, want %q", got, installedBinary)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0100 == 0 {
		t.Errorf("expected the executable bit, got %v", info.Mode())
	}
}

func TestUpgradeWindowsZipAsset(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "windows-amd64.zip", entryName: "nga.exe"})
	target := newTestBinary(t, "nga.exe")

	res, err := fx.updater(target, "windows", "amd64").
		Upgrade(context.Background(), "v0.8.0", Options{Apply: true, Verify: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.AssetName != "nga_v0.9.0_windows-amd64.zip" {
		t.Errorf("asset = %q", res.AssetName)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != installedBinary {
		t.Errorf("binary content = %q, want %q", got, installedBinary)
	}
}

func TestUpgradeChecksumMismatchKeepsBinary(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "linux-amd64.tar.gz", entryName: "nga", badChecksum: true})
	target := newTestBinary(t, "nga")

	_, err := fx.updater(target, "linux", "amd64").
		Upgrade(context.Background(), "v0.1.0", Options{Apply: true, Verify: true})
	if err == nil {
		t.Fatal("expected a checksum mismatch to abort the upgrade")
	}
	if !strings.Contains(err.Error(), "SHA256") {
		t.Errorf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(target)
	if string(got) != "old-binary" {
		t.Errorf("binary must stay untouched, got %q", got)
	}
}

func TestUpgradeMissingChecksumsAborts(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "linux-amd64.tar.gz", entryName: "nga", noChecksumAsset: true})
	target := newTestBinary(t, "nga")
	u := fx.updater(target, "linux", "amd64")

	if _, err := u.Upgrade(context.Background(), "v0.1.0", Options{Apply: true, Verify: true}); err == nil {
		t.Fatal("expected a missing checksums.txt to abort the upgrade")
	}

	// With verification explicitly opted out, the same release installs fine.
	res, err := u.Upgrade(context.Background(), "v0.1.0", Options{Apply: true, Verify: false})
	if err != nil {
		t.Fatalf("unexpected error with --no-checksum: %v", err)
	}
	if res.ChecksumVerified {
		t.Errorf("checksum_verified should be false")
	}
	got, _ := os.ReadFile(target)
	if string(got) != installedBinary {
		t.Errorf("binary content = %q", got)
	}
}

func TestUpgradeCheckOnlyDoesNotTouchBinary(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "linux-amd64.tar.gz", entryName: "nga"})
	target := newTestBinary(t, "nga")

	res, err := fx.updater(target, "linux", "amd64").
		Upgrade(context.Background(), "v0.1.0", Options{Apply: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.UpdateAvailable || res.Applied {
		t.Errorf("check-only must not apply: %+v", res)
	}

	got, _ := os.ReadFile(target)
	if string(got) != "old-binary" {
		t.Errorf("binary must stay untouched, got %q", got)
	}
}

func TestUpgradeUpToDateDowngradeAndForce(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "linux-amd64.tar.gz", entryName: "nga"})

	cases := []struct {
		name        string
		current     string
		force       bool
		wantApplied bool
		wantMsg     string
	}{
		{"same version", "v0.9.0", false, false, "已是最新"},
		{"newer local build", "v1.0.0", false, false, "未降级"},
		{"forced reinstall", "v0.9.0", true, true, "已重新安装"},
		{"forced downgrade", "v1.1.0", true, true, "已降级至"},
		{"dev build upgrades", "dev", false, true, "已从"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target := newTestBinary(t, "nga")
			res, err := fx.updater(target, "linux", "amd64").
				Upgrade(context.Background(), c.current, Options{Apply: true, Verify: true, Force: c.force})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Applied != c.wantApplied {
				t.Errorf("applied = %v, want %v (%s)", res.Applied, c.wantApplied, res.Message)
			}
			if !strings.Contains(res.Message, c.wantMsg) {
				t.Errorf("message = %q, want it to contain %q", res.Message, c.wantMsg)
			}
		})
	}
}

func TestUpgradeTargetTagUsesTagsEndpoint(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.3.0", platform: "linux-amd64.tar.gz", entryName: "nga"})
	target := newTestBinary(t, "nga")

	res, err := fx.updater(target, "linux", "amd64").
		Upgrade(context.Background(), "v0.9.0", Options{Apply: true, Verify: true, Force: true, TargetTag: "v0.3.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TargetVersion != "v0.3.0" {
		t.Errorf("target = %q, want v0.3.0", res.TargetVersion)
	}
}

func TestUpgradeUnknownTagReportsNotFound(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "linux-amd64.tar.gz", entryName: "nga"})
	target := newTestBinary(t, "nga")

	_, err := fx.updater(target, "linux", "amd64").
		Upgrade(context.Background(), "v0.1.0", Options{Apply: true, Verify: true, TargetTag: "v9.9.9"})
	if err == nil {
		t.Fatal("expected an error for an unknown release tag")
	}
	if !strings.Contains(err.Error(), "未找到发布版本") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpgradeMissingPlatformAsset(t *testing.T) {
	fx := newReleaseFixture(t, fixtureOpts{tag: "v0.9.0", platform: "linux-amd64.tar.gz", entryName: "nga"})
	target := newTestBinary(t, "nga")

	_, err := fx.updater(target, "linux", "arm64").
		Upgrade(context.Background(), "v0.1.0", Options{Apply: true, Verify: true})
	if err == nil {
		t.Fatal("expected an error when the platform asset is missing")
	}
	if !strings.Contains(err.Error(), "nga_v0.9.0_linux-arm64.tar.gz") {
		t.Errorf("error should name the expected asset: %v", err)
	}
}

func TestReplaceBinaryOverwritesInPlace(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "next-nga")
	dst := filepath.Join(dir, "nga")
	if err := os.WriteFile(src, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source should be consumed by the rename")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("dst = %q, want new", got)
	}
}

func TestTargetPathFallsBackToRunningExecutable(t *testing.T) {
	if got := New(Config{}).targetPath(); got == "" {
		t.Skip("os.Executable is unavailable in this harness")
	}
}

// --- review regression tests ------------------------------------------

func clearProxyEnv(t *testing.T) {
	t.Helper()

	// The default transport honours ProxyFromEnvironment, which would route
	// httptest traffic through a real proxy when one is configured.
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
}

func delayed(handler http.HandlerFunc, d time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(d):
		case <-r.Context().Done():
			return
		}
		handler(w, r)
	}
}

// A slow asset transfer must not be cut short by the release-API deadline.
func TestSlowDownloadIsNotCappedByAPITimeout(t *testing.T) {
	clearProxyEnv(t)

	tag := "v0.9.0"
	platform := "linux-amd64.tar.gz"
	assetName := "nga_" + tag + "_" + platform
	archive := tarGzWithBinary(t, "nga", []byte(installedBinary))

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/fimreal/nga-cli/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, releasePayload("http://"+r.Host, tag, assetName, false))
	})
	// Headers go out first, then the body trickles in: exactly the case a
	// global Client.Timeout kills mid-transfer.
	mux.HandleFunc("/files/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(archive)))
		w.WriteHeader(http.StatusOK)
		mid := len(archive) / 2
		if _, err := w.Write(archive[:mid]); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-time.After(120 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(archive[mid:])
	})
	mux.HandleFunc("/files/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", digestOf(archive), assetName)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	target := newTestBinary(t, "nga")
	u := New(Config{
		APIBase:    srv.URL,
		GOOS:       "linux",
		GOARCH:     "amd64",
		TargetPath: target,
		Timeout:    30 * time.Millisecond,
	})

	res, err := u.Upgrade(context.Background(), "v0.1.0", Options{Apply: true, Verify: true})
	if err != nil {
		t.Fatalf("download exceeded the release-API deadline: %v", err)
	}
	if !res.Applied || !res.ChecksumVerified {
		t.Errorf("unexpected result: %+v", res)
	}

	// The deadline still bounds the release-API request itself.
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/repos/fimreal/nga-cli/releases/latest", delayed(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, releasePayload("http://"+r.Host, tag, assetName, false))
	}, 300*time.Millisecond))
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()

	u2 := New(Config{APIBase: srv2.URL, TargetPath: target, Timeout: 30 * time.Millisecond})
	if _, err := u2.Upgrade(context.Background(), "v0.1.0", Options{Apply: false}); err == nil {
		t.Fatal("expected the release-API deadline to abort a stalled metadata request")
	} else if !strings.Contains(err.Error(), "deadline") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewBuildsClientWithoutGlobalTimeout(t *testing.T) {
	if got := New(Config{}).client.Timeout; got != 0 {
		t.Errorf("client.Timeout = %v, want 0 so streamed downloads are context-bounded only", got)
	}
}

func TestTargetPathResolvesConfiguredSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "nga")
	if err := os.WriteFile(real, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "nga-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	// Replacing through the link must target the installation, not the link.
	got := New(Config{TargetPath: link}).targetPath()
	if want, err := filepath.EvalSymlinks(real); err == nil {
		if got != want {
			t.Errorf("targetPath() = %q, want %q", got, want)
		}
	} else {
		t.Fatal(err)
	}
}

func TestFirstLineTruncatesOnRuneBoundary(t *testing.T) {
	long := strings.Repeat("校验失败", 100) + "\nignored"
	got := firstLine(long)
	if !utf8.ValidString(got) {
		t.Errorf("truncated text is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected an ellipsis marker, got %q", got)
	}
	if len([]rune(got)) != 201 {
		t.Errorf("rune count = %d, want 201 (200 + ellipsis)", len([]rune(got)))
	}

	if short := firstLine("  错误信息\nrest"); short != "错误信息" {
		t.Errorf("firstLine = %q", short)
	}
}
