package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	doctorTargetTID int64
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "检测网络连通性、代理状态与 NGA WAF 防风控有效性",
	Long:  "对 NGA 核心域名 (ngabbs.com, bbs.nga.cn) 进行 DNS 解析与连通性测试，模拟轻量级移动端请求以验证是否触发 WAF 阻断，辅助 Homelab 与服务器网络调优。",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, err := getClient()
		if err != nil {
			return fmt.Errorf("初始化诊断客户端失败: %w", err)
		}

		ctx := context.Background()
		report, err := cli.DoctorProbe(ctx, doctorTargetTID)
		if err != nil {
			return fmt.Errorf("诊断执行失败: %w", err)
		}

		if flagJSON {
			return outputJSON(report)
		}

		PrintDoctorReport(report)
		return nil
	},
}

func init() {
	doctorCmd.Flags().Int64Var(&doctorTargetTID, "target", 38888888, "用于探测连通性的测试帖子 TID")
	RootCmd.AddCommand(doctorCmd)
}
