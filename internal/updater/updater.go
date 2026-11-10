package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"nga/pkg/model"
)

// ErrReleaseNotFound means the repository or tag being queried publishes no release.
var ErrReleaseNotFound = errors.New("未找到发布版本")

const (
	// DefaultRepo is the GitHub repository publishing release assets.
	DefaultRepo = "fimreal/nga-cli"
	// DefaultAPIBase is the GitHub REST API root.
	DefaultAPIBase    = "https://api.github.com"
	checksumAsset     = "checksums.txt"
	binaryBase        = "nga"
	defaultAPITimeout = 60 * time.Second
	downloadTimeout   = 10 * time.Minute
	jsonAcceptHeader  = "application/vnd.github+json"
)

// Asset is a release attachment as reported by the GitHub Release API.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// Release is the subset of the GitHub Release API payload this package needs.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []Asset   `json:"assets"`
}

// Config describes where to look for updates and what to update.
type Config struct {
	Repo      string
	APIBase   string
	ProxyURL  string
	Token     string
	UserAgent string
	// Timeout bounds a single release-API request. Asset downloads are not
	// covered by it; they use their own, longer deadline.
	Timeout    time.Duration
	GOOS       string
	GOARCH     string
	TargetPath string
	Client     *http.Client
}

// Options controls a single upgrade run.
type Options struct {
	// TargetTag pins a specific release; empty means the latest one.
	TargetTag string
	// Apply installs the binary; false only reports availability.
	Apply bool
	// Force installs even when the current version is equal or newer.
	Force bool
	// Verify requires the release checksums.txt entry (default true).
	Verify bool
}

// Updater checks GitHub Releases and replaces the running binary in place.
type Updater struct {
	cfg     Config
	client  *http.Client
	apiBase string
	repo    string
	goos    string
	goarch  string
}

// New creates an Updater, filling in defaults for empty config fields.
func New(cfg Config) *Updater {
	if cfg.Repo == "" {
		cfg.Repo = DefaultRepo
	}
	if cfg.APIBase == "" {
		cfg.APIBase = DefaultAPIBase
	}
	if cfg.GOOS == "" {
		cfg.GOOS = runtime.GOOS
	}
	if cfg.GOARCH == "" {
		cfg.GOARCH = runtime.GOARCH
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultAPITimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "nga-cli-updater"
	}

	client := cfg.Client
	if client == nil {
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          5,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
		if cfg.ProxyURL != "" {
			if pURL, err := url.Parse(cfg.ProxyURL); err == nil {
				transport.Proxy = http.ProxyURL(pURL)
			}
		}
		// No Client.Timeout: it also caps time spent reading resp.Body, which
		// would cut off multi-megabyte asset transfers. Every request carries
		// its own context deadline instead.
		client = &http.Client{Transport: transport}
	}

	return &Updater{
		cfg:     cfg,
		client:  client,
		apiBase: strings.TrimRight(cfg.APIBase, "/"),
		repo:    strings.Trim(cfg.Repo, "/"),
		goos:    cfg.GOOS,
		goarch:  cfg.GOARCH,
	}
}

// AssetName returns the release asset name matching a tag and the target platform.
func (u *Updater) AssetName(tag string) string {
	ext := ".tar.gz"
	if u.goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("nga_%s_%s%s", tag, u.platformSlug(), ext)
}

func (u *Updater) platformSlug() string {
	arch := u.goarch
	if u.goos == "linux" && arch == "arm" {
		arch = "armv7"
	}
	return u.goos + "-" + arch
}

// ReleaseFor resolves the requested release, defaulting to the latest one.
func (u *Updater) ReleaseFor(ctx context.Context, tag string) (*Release, error) {
	if tag == "" {
		return u.fetchRelease(ctx, u.apiBase+"/repos/"+u.repo+"/releases/latest")
	}

	rel, err := u.fetchRelease(ctx, u.apiBase+"/repos/"+u.repo+"/releases/tags/"+url.PathEscape(tag))
	if err == nil {
		return rel, nil
	}
	// Tags here follow the v-prefixed convention, but users often omit it.
	if !strings.HasPrefix(tag, "v") && errors.Is(err, ErrReleaseNotFound) {
		return u.fetchRelease(ctx, u.apiBase+"/repos/"+u.repo+"/releases/tags/"+url.PathEscape("v"+tag))
	}
	return nil, err
}

func (u *Updater) fetchRelease(ctx context.Context, endpoint string) (*Release, error) {
	actx, cancel := context.WithTimeout(ctx, u.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(actx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", jsonAcceptHeader)
	if u.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.cfg.Token)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub Release 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 GitHub Release 响应失败: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w (仓库 %s 是否存在该 Release？)", ErrReleaseNotFound, u.repo)
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("GitHub API 限流 (HTTP %d)，请配置 GITHUB_TOKEN 或更换网络后重试", resp.StatusCode)
		}
		return nil, fmt.Errorf("GitHub API 返回 HTTP %d: %s", resp.StatusCode, firstLine(string(body)))
	}

	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 响应失败: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("GitHub Release 响应缺少 tag_name 字段")
	}
	return &rel, nil
}

// Upgrade checks the release channel and, when opts.Apply is set, replaces the
// running binary with the matching asset from that release.
func (u *Updater) Upgrade(ctx context.Context, current string, opts Options) (*model.UpgradeResult, error) {
	rel, err := u.ReleaseFor(ctx, strings.TrimSpace(opts.TargetTag))
	if err != nil {
		return nil, err
	}

	res := &model.UpgradeResult{
		CurrentVersion: current,
		TargetVersion:  rel.TagName,
		ReleaseName:    rel.Name,
		ReleaseURL:     rel.HTMLURL,
		TargetPath:     u.targetPath(),
	}
	if !rel.PublishedAt.IsZero() {
		res.PublishedAt = rel.PublishedAt.UTC().Format(time.RFC3339)
	}

	asset, err := u.pickAsset(rel)
	if err != nil {
		return nil, err
	}
	res.AssetName = asset.Name
	res.AssetSize = asset.Size
	cmp := compareVersion(current, rel.TagName)
	res.UpdateAvailable = cmp < 0

	if !opts.Apply {
		if res.UpdateAvailable {
			res.Message = fmt.Sprintf("发现新版本 %s (当前 %s)，执行 nga upgrade 完成升级", rel.TagName, displayVersion(current))
		} else {
			res.Message = fmt.Sprintf("当前版本 %s 已是最新", displayVersion(current))
		}
		return res, nil
	}

	if !res.UpdateAvailable && !opts.Force {
		if cmp > 0 {
			res.Message = fmt.Sprintf("当前版本 %s 高于目标 %s，未降级；如需降级请追加 --force", displayVersion(current), rel.TagName)
			return res, nil
		}
		res.Message = fmt.Sprintf("当前版本 %s 已是最新，未重复安装；如需覆盖安装请追加 --force", displayVersion(current))
		return res, nil
	}

	if err := u.apply(ctx, rel, asset, res, opts.Verify); err != nil {
		return res, err
	}
	res.Applied = true
	switch {
	case cmp > 0:
		res.Message = fmt.Sprintf("已降级至 %s (%s)", rel.TagName, res.TargetPath)
	case cmp == 0:
		res.Message = fmt.Sprintf("已重新安装 %s (%s)", rel.TagName, res.TargetPath)
	default:
		res.Message = fmt.Sprintf("已从 %s 升级到 %s (%s)", displayVersion(current), rel.TagName, res.TargetPath)
	}
	return res, nil
}

func (u *Updater) pickAsset(rel *Release) (*Asset, error) {
	want := u.AssetName(rel.TagName)
	for i := range rel.Assets {
		if strings.EqualFold(rel.Assets[i].Name, want) {
			return &rel.Assets[i], nil
		}
	}

	names := make([]string, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		names = append(names, a.Name)
	}
	return nil, fmt.Errorf("Release %s 未包含当前平台 (%s/%s) 的资产 %q，可用资产: %s",
		rel.TagName, u.goos, u.goarch, want, strings.Join(names, ", "))
}

func (u *Updater) apply(ctx context.Context, rel *Release, asset *Asset, res *model.UpgradeResult, verify bool) error {
	target := res.TargetPath
	if target == "" {
		return fmt.Errorf("无法定位当前可执行文件路径，请手动替换，或用 --bin 指定二进制路径")
	}
	dir := filepath.Dir(target)
	work, err := os.MkdirTemp(dir, ".nga-upgrade-")
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("目标目录 %s 不可写，请使用 sudo 运行 nga upgrade，或改用 --bin 指向用户目录下可写的位置", dir)
		}
		return fmt.Errorf("在 %s 创建临时目录失败: %w", dir, err)
	}
	defer os.RemoveAll(work)

	// asset.Name comes from the API response, so only its base name is trusted.
	archivePath := filepath.Join(work, filepath.Base(strings.ReplaceAll(asset.Name, "\\", "/")))
	downloaded, err := u.download(ctx, asset.BrowserDownloadURL, archivePath)
	if err != nil {
		return err
	}
	res.AssetSize = downloaded

	if verify {
		digest, err := u.verifyChecksum(ctx, rel, asset.Name, archivePath)
		if err != nil {
			return err
		}
		res.Checksum = digest
		res.ChecksumVerified = true
	}

	binName := binaryBase
	if u.goos == "windows" {
		binName += ".exe"
	}
	newBinary := filepath.Join(work, "next-"+binName)
	if err := extractBinary(archivePath, binName, newBinary); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	if err := replaceBinary(newBinary, target); err != nil {
		return fmt.Errorf("替换二进制失败: %w", err)
	}
	return nil
}

func (u *Updater) verifyChecksum(ctx context.Context, rel *Release, assetName, archivePath string) (string, error) {
	sums, err := u.fetchChecksums(ctx, rel)
	if err != nil {
		return "", err
	}
	if len(sums) == 0 {
		return "", fmt.Errorf("checksums.txt 为空或格式无法识别；如确认来源可信可追加 --no-checksum 跳过校验")
	}

	want, ok := sums[strings.ToLower(assetName)]
	if !ok {
		return "", fmt.Errorf("checksums.txt 未登记资产 %q；如确认来源可信可追加 --no-checksum 跳过校验", assetName)
	}

	got, err := sha256File(archivePath)
	if err != nil {
		return "", fmt.Errorf("计算本地摘要失败: %w", err)
	}
	if !strings.EqualFold(got, want) {
		return "", fmt.Errorf("SHA256 校验失败: 期望 %s，实际 %s，已中止升级", want, got)
	}
	return want, nil
}

func (u *Updater) fetchChecksums(ctx context.Context, rel *Release) (map[string]string, error) {
	var asset *Asset
	for i := range rel.Assets {
		if strings.EqualFold(rel.Assets[i].Name, checksumAsset) {
			asset = &rel.Assets[i]
			break
		}
	}
	if asset == nil {
		return nil, fmt.Errorf("Release %s 未提供 %s，无法校验完整性；如确认来源可信可追加 --no-checksum 跳过校验",
			rel.TagName, checksumAsset)
	}

	tmp, err := os.MkdirTemp("", "nga-sum-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	path := filepath.Join(tmp, checksumAsset)
	if _, err := u.download(ctx, asset.BrowserDownloadURL, path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseChecksums(data), nil
}

func (u *Updater) download(ctx context.Context, rawURL, dst string) (int64, error) {
	dctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", u.cfg.UserAgent)
	if u.cfg.Token != "" && req.URL.Hostname() == "api.github.com" {
		req.Header.Set("Authorization", "Bearer "+u.cfg.Token)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("下载 %s 失败: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("下载 %s 返回 HTTP %d", assetBase(rawURL), resp.StatusCode)
	}

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, resp.Body)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return n, fmt.Errorf("下载 %s 中断: %w", assetBase(rawURL), err)
	}
	return n, nil
}

// targetPath resolves the binary to replace, following symlinks so that an
// installation is replaced instead of a link that merely points at it.
func (u *Updater) targetPath() string {
	path := u.cfg.TargetPath
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			return ""
		}
		path = exe
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// replaceBinary swaps the new binary into place atomically where possible.
func replaceBinary(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	// Windows keeps a running executable locked: move it aside first.
	backup := dst + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(dst, backup); err != nil {
		return fmt.Errorf("当前二进制被占用，请关闭其他 nga 实例后重试 (%w)", err)
	}
	if err := os.Rename(src, dst); err != nil {
		_ = os.Rename(backup, dst)
		return err
	}
	return nil
}

func assetBase(rawURL string) string {
	return filepath.Base(strings.Split(rawURL, "?")[0])
}

func displayVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	// Truncate by rune so multi-byte error text is never cut mid-character.
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}
