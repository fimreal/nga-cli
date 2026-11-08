package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"nga/internal/profile"
)

var (
	profileSetUID   int64
	profileSetCID   string
	profileSetProxy string
	profileSetUA    string
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "管理多账号 Profile 配置（支持在不同 Cookie 凭证与代理间切换）",
	Long:  "提供多账号 Profile 管理功能，支持针对不同板块或角色快速切换凭证、代理与 User-Agent。",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfileList()
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有配置的账号 Profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfileList()
	},
}

var profileUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "切换当前默认活跃的 Profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		mgr, err := getProfileManager()
		if err != nil {
			return err
		}

		if err := mgr.Use(name); err != nil {
			return err
		}

		fmt.Printf("已切换当前活跃 Profile 为: %s\n", name)
		return nil
	},
}

var profileSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "新增或更新指定的 Profile 凭据配置",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		mgr, err := getProfileManager()
		if err != nil {
			return err
		}

		// Read existing if any
		p, _ := mgr.Get(name)
		p.Name = name

		if profileSetUID > 0 {
			p.CookieUID = profileSetUID
		}
		if profileSetCID != "" {
			p.CookieCID = profileSetCID
		}
		if cmd.Flags().Changed("proxy") {
			p.Proxy = profileSetProxy
		}
		if cmd.Flags().Changed("ua") {
			p.UserAgent = profileSetUA
		}

		if err := mgr.Set(p); err != nil {
			return fmt.Errorf("保存 Profile 失败: %w", err)
		}

		fmt.Printf("Profile %q 配置保存成功。\n", name)
		return nil
	},
}

var profileRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "删除指定的 Profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		mgr, err := getProfileManager()
		if err != nil {
			return err
		}

		if err := mgr.Remove(name); err != nil {
			return fmt.Errorf("删除 Profile 失败: %w", err)
		}

		fmt.Printf("Profile %q 已删除。\n", name)
		return nil
	},
}

func runProfileList() error {
	mgr, err := getProfileManager()
	if err != nil {
		return err
	}

	profiles, active := mgr.List()
	if len(profiles) == 0 {
		fmt.Println("（尚未配置任何 Profile，可通过 'nga profile set <name> --uid <uid> --cid <cid>' 添加）")
		return nil
	}

	divider := strings.Repeat("-", 70)
	fmt.Println(divider)
	fmt.Printf("%-3s %-12s %-12s %-25s\n", "状态", "Profile", "UID", "代理出口")
	fmt.Println(divider)

	for _, p := range profiles {
		status := "   "
		if p.Name == active {
			status = " * "
		}
		proxyStr := p.Proxy
		if proxyStr == "" {
			proxyStr = "直连"
		}
		fmt.Printf("%-3s %-12s %-12d %-25s\n", status, p.Name, p.CookieUID, proxyStr)
	}
	fmt.Println(divider)
	fmt.Printf("当前活跃 Profile: %s (可通过全局参数 --profile <name> 临时切换)\n", active)
	return nil
}

func getProfileManager() (*profile.Manager, error) {
	configDir := viper.GetString("cache_dir")
	var path string
	if configDir != "" {
		path = configDir + "/profiles.yaml"
	}
	return profile.NewManager(path)
}

func init() {
	profileSetCmd.Flags().Int64Var(&profileSetUID, "uid", 0, "NGA 登录 UID")
	profileSetCmd.Flags().StringVar(&profileSetCID, "cid", "", "NGA 登录 CID 凭证")
	profileSetCmd.Flags().StringVar(&profileSetProxy, "proxy", "", "此 Profile 专属代理")
	profileSetCmd.Flags().StringVar(&profileSetUA, "ua", "", "此 Profile 自定义 User-Agent")

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileUseCmd)
	profileCmd.AddCommand(profileSetCmd)
	profileCmd.AddCommand(profileRemoveCmd)

	RootCmd.AddCommand(profileCmd)
}
