package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	checkInForce bool
	checkInStat  bool
)

var checkInCmd = &cobra.Command{
	Use:     "checkin",
	Aliases: []string{"check-in", "signin"},
	Short:   "执行 NGA 论坛每日签到与打卡（支持防重复打卡与状态查询）",
	Long: `向 NGA 论坛发起每日签到请求，领取论坛打卡奖励与金币。
默认结合本地 SQLite 缓存进行当日已签到拦截，避免多次无效网络请求；可使用 --force 强制发起网络签到；或使用 --stat 查询签到统计。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, err := getClient()
		if err != nil {
			return fmt.Errorf("初始化客户端失败: %w", err)
		}

		ctx := context.Background()

		if checkInStat {
			stat, err := cli.GetCheckInStat(ctx)
			if err != nil {
				return err
			}

			if flagJSON {
				return outputJSON(stat)
			}

			divider := strings.Repeat("=", 80)
			fmt.Println(divider)
			fmt.Println("📊 NGA 每日签到统计")
			fmt.Printf("账号 UID: %d\n", stat.UID)
			fmt.Printf("统计信息: %s\n", stat.Stat)
			fmt.Println(divider)
			return nil
		}

		result, err := cli.CheckIn(ctx, checkInForce)
		if err != nil {
			return err
		}

		if flagJSON {
			return outputJSON(result)
		}

		divider := strings.Repeat("=", 80)
		fmt.Println(divider)
		if result.AlreadyDone {
			fmt.Println("ℹ️  NGA 每日签到（今日已完成）")
		} else {
			fmt.Println("✅ NGA 每日签到成功！")
		}
		fmt.Printf("账号 UID: %d\n", result.UID)
		fmt.Printf("签到日期: %s\n", result.Date)
		fmt.Printf("处理结果: %s\n", result.Message)
		fmt.Println(divider)
		return nil
	},
}

func init() {
	checkInCmd.Flags().BoolVar(&checkInForce, "force", false, "跳过本地今日已签到拦截，强制发起网络签到请求")
	checkInCmd.Flags().BoolVar(&checkInStat, "stat", false, "查询签到统计信息（如连续签到天数等）")

	RootCmd.AddCommand(checkInCmd)
}
