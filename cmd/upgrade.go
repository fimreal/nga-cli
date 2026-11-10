package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"nga/internal/updater"
)

var (
	upgradeCheck      bool
	upgradeTag        string
	upgradeForce      bool
	upgradeNoChecksum bool
	upgradeRepo       string
	upgradeAPIBase    string
	upgradeBin        string
)

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Aliases: []string{"update", "self-update"},
	Short:   "从 GitHub Release 检测并下载最新版本，实现客户端自更新",
	Long: `查询 GitHub Release 最新（或 --version 指定）版本，比对本地二进制版本号，
自动下载当前平台对应的预编译资产、按 checksums.txt 校验 SHA256，并原子替换本地二进制。

替换失败或权限不足时二进制保持原样；如需降级到历史版本：nga upgrade --version v0.1.0 --force。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := firstConfigured(upgradeRepo, viper.GetString("upgrade_repo"))
		apiBase := firstConfigured(upgradeAPIBase, viper.GetString("upgrade_api_base"))

		cfg := updater.Config{
			Repo:       repo,
			APIBase:    apiBase,
			ProxyURL:   resolveProxyURL(),
			Token:      os.Getenv("GITHUB_TOKEN"),
			UserAgent:  fmt.Sprintf("nga-cli/%s", Version),
			TargetPath: upgradeBin,
		}

		opts := updater.Options{
			TargetTag: upgradeTag,
			Apply:     !upgradeCheck,
			Force:     upgradeForce,
			Verify:    !upgradeNoChecksum,
		}

		res, err := updater.New(cfg).Upgrade(context.Background(), Version, opts)
		if err != nil {
			if res != nil && flagJSON {
				_ = outputJSON(res)
			}
			return err
		}

		if flagJSON {
			return outputJSON(res)
		}

		PrintUpgradeResult(res)
		return nil
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheck, "check", false, "仅检测是否存在新版本，不下载也不替换二进制")
	upgradeCmd.Flags().StringVar(&upgradeTag, "version", "", "指定要安装的目标版本 Tag (默认为最新 Release)")
	upgradeCmd.Flags().BoolVar(&upgradeForce, "force", false, "目标版本与当前一致或更旧时仍然执行替换 (用于重装或降级)")
	upgradeCmd.Flags().BoolVar(&upgradeNoChecksum, "no-checksum", false, "跳过 checksums.txt 的 SHA256 校验 (不建议)")
	upgradeCmd.Flags().StringVar(&upgradeRepo, "repo", "", "覆盖 GitHub 仓库 owner/name (默认: "+updater.DefaultRepo+")")
	upgradeCmd.Flags().StringVar(&upgradeAPIBase, "api-base", "", "覆盖 GitHub API 地址 (用于 GitHub Enterprise 或镜像站)")
	upgradeCmd.Flags().StringVar(&upgradeBin, "bin", "", "指定被替换的二进制路径 (默认为当前运行的可执行文件，版本比对仍以当前运行版本为准)")

	RootCmd.AddCommand(upgradeCmd)
}

// firstConfigured returns the first non-empty value, flag before config file.
func firstConfigured(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
