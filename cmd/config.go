package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "查看或修改 NGA CLI 配置",
	Long:  "管理保存在本地配置文件中的 NGA CLI 运行参数，包括 Cookie、代理与速率限制。",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示当前所有配置项",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings := viper.AllSettings()
		data, err := yaml.Marshal(settings)
		if err != nil {
			return err
		}
		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			fmt.Println("（未加载外部配置文件，当前使用默认或环境变量设置）")
		} else {
			fmt.Printf("配置文件路径: %s\n\n", configFile)
		}
		fmt.Println(string(data))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "设置配置项并持久化保存到配置文件",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		val := args[1]

		viper.Set(key, val)

		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			configDir := filepath.Join(home, ".config", "nga")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return err
			}
			configFile = filepath.Join(configDir, "config.yaml")
		}

		if err := viper.WriteConfigAs(configFile); err != nil {
			// If already exists, write directly
			if err := viper.WriteConfig(); err != nil {
				return fmt.Errorf("保存配置失败: %w", err)
			}
		}

		fmt.Printf("已更新配置 %s = %s (保存至 %s)\n", key, val, configFile)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	RootCmd.AddCommand(configCmd)
}
