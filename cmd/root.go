package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"nga/internal/cache"
	"nga/internal/client"
)

var (
	cfgFile     string
	flagProxy   string
	flagUID     int64
	flagCID     string
	flagUA      string
	flagRateMs   int
	flagJSON     bool
	flagNoColor  bool
	flagNoCache  bool
	flagCacheDir string
	flagProfile  string
)

// RootCmd represents the base command when called without any subcommands.
var RootCmd = &cobra.Command{
	Use:   "nga",
	Short: "NGA 论坛命令行交互工具 (CLI)",
	Long: `NGA CLI 是一个轻量、高效、低流量消耗的艾泽拉斯国家地理论坛 (NGA) 终端工具。
支持帖子增量读取、Tail 尾部查看、UID 过滤、楼主模式、反风控伪装以及网络连通性诊断。`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认: ~/.config/nga/config.yaml)")
	RootCmd.PersistentFlags().StringVar(&flagProxy, "proxy", "", "HTTP / SOCKS5 代理地址 (例如 http://127.0.0.1:7890)")
	RootCmd.PersistentFlags().Int64Var(&flagUID, "cookie-uid", 0, "NGA 登录 UID (ngaPassportUid)")
	RootCmd.PersistentFlags().StringVar(&flagCID, "cookie-cid", "", "NGA 登录凭证 (ngaPassportCid)")
	RootCmd.PersistentFlags().StringVar(&flagUA, "user-agent", "", "自定义 User-Agent (默认采用移动端伪装)")
	RootCmd.PersistentFlags().IntVar(&flagRateMs, "rate-limit", 1500, "最小请求间隔毫秒数 (默认 1500ms)")
	RootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "以结构化 JSON 格式输出结果")
	RootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "关闭彩色与装饰字符")
	RootCmd.PersistentFlags().BoolVar(&flagNoCache, "no-cache", false, "禁用本地 SQLite 缓存，强制拉取网络数据")
	RootCmd.PersistentFlags().StringVar(&flagCacheDir, "cache-dir", "", "本地 SQLite 缓存目录 (默认: ~/.cache/nga)")
	RootCmd.PersistentFlags().StringVar(&flagProfile, "profile", "", "指定本次运行使用的账号 Profile")

	viper.BindPFlag("proxy", RootCmd.PersistentFlags().Lookup("proxy"))
	viper.BindPFlag("cookie_uid", RootCmd.PersistentFlags().Lookup("cookie-uid"))
	viper.BindPFlag("cookie_cid", RootCmd.PersistentFlags().Lookup("cookie-cid"))
	viper.BindPFlag("user_agent", RootCmd.PersistentFlags().Lookup("user-agent"))
	viper.BindPFlag("rate_limit", RootCmd.PersistentFlags().Lookup("rate-limit"))
	viper.BindPFlag("no_cache", RootCmd.PersistentFlags().Lookup("no-cache"))
	viper.BindPFlag("cache_dir", RootCmd.PersistentFlags().Lookup("cache-dir"))
	viper.BindPFlag("profile", RootCmd.PersistentFlags().Lookup("profile"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			configDir := filepath.Join(home, ".config", "nga")
			viper.AddConfigPath(configDir)
		}
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.SetEnvPrefix("NGA")
	viper.AutomaticEnv()

	// Read in config file if available
	_ = viper.ReadInConfig()
}

// getClient creates an initialized client using configuration flags and environment.
func getClient() (*client.Client, error) {
	proxy := viper.GetString("proxy")
	if proxy == "" {
		proxy = os.Getenv("ALL_PROXY")
	}
	if proxy == "" {
		proxy = os.Getenv("HTTPS_PROXY")
	}
	if proxy == "" {
		proxy = os.Getenv("HTTP_PROXY")
	}

	uid := viper.GetInt64("cookie_uid")
	if uid == 0 && os.Getenv("NGA_UID") != "" {
		uid, _ = strconv.ParseInt(os.Getenv("NGA_UID"), 10, 64)
	}

	cid := viper.GetString("cookie_cid")
	if cid == "" {
		cid = os.Getenv("NGA_CID")
	}

	ua := viper.GetString("user_agent")
	rateLimit := viper.GetInt("rate_limit")
	if rateLimit <= 0 {
		rateLimit = 1500
	}

	// Profile resolution: fallback to profile if flag/config doesn't explicitly provide creds
	profileName := flagProfile
	if profileName == "" {
		profileName = viper.GetString("profile")
	}
	if mgr, err := getProfileManager(); err == nil {
		if p, ok := mgr.Get(profileName); ok {
			if uid == 0 && p.CookieUID > 0 {
				uid = p.CookieUID
			}
			if cid == "" && p.CookieCID != "" {
				cid = p.CookieCID
			}
			if proxy == "" && p.Proxy != "" {
				proxy = p.Proxy
			}
			if ua == "" && p.UserAgent != "" {
				ua = p.UserAgent
			}
		}
	}

	noCache := flagNoCache || viper.GetBool("no_cache")
	var cacheStore *cache.Store
	if !noCache {
		cDir := viper.GetString("cache_dir")
		if store, err := cache.New(cDir); err == nil {
			cacheStore = store
		}
	}

	cfg := client.Config{
		ProxyURL:    proxy,
		CookieUID:   uid,
		CookieCID:   cid,
		UserAgent:   ua,
		RateLimitMs: rateLimit,
		CacheStore:  cacheStore,
		NoCache:     noCache,
	}

	return client.New(cfg)
}

// outputJSON writes any object as formatted JSON to stdout.
func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func parseUIDList(s string) []int64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var res []int64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			res = append(res, n)
		}
	}
	return res
}
