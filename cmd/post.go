package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"nga/internal/client"
)

var (
	postSubject string
	postMessage string
	postFile    string
	postForce   bool
)

var postCmd = &cobra.Command{
	Use:   "post <fid>",
	Short: "在指定板块发表新主题帖",
	Long:  "向目标板块 FID 发布新主题，支持标题、正文直接输入或从文件导入，内置防重保护机制。",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("无效的板块 FID: %s", args[0])
		}

		subject := strings.TrimSpace(postSubject)
		if subject == "" {
			return fmt.Errorf("请通过 -t/--title 指定主贴标题")
		}

		var content string
		if postFile != "" {
			data, err := os.ReadFile(postFile)
			if err != nil {
				return fmt.Errorf("读取主贴内容文件失败: %w", err)
			}
			content = string(data)
		} else if postMessage != "" {
			content = postMessage
		} else {
			return fmt.Errorf("请通过 -m/--message 或 -f/--file 提供主贴正文")
		}

		content = strings.TrimSpace(content)
		if content == "" {
			return fmt.Errorf("主贴正文不能为空")
		}

		cli, err := getClient()
		if err != nil {
			return fmt.Errorf("初始化客户端失败: %w", err)
		}

		ctx := context.Background()
		opts := client.NewThreadOptions{
			Fid:     fid,
			Subject: subject,
			Content: content,
			Force:   postForce,
		}

		res, err := cli.PostNewThread(ctx, opts)
		if err != nil {
			return err
		}

		if flagJSON {
			return outputJSON(res)
		}

		fmt.Println("================================================================================")
		fmt.Printf("✅ 发帖成功！板块 FID: %d | 标题: %s\n", fid, subject)
		fmt.Printf("响应结果: %s\n", res.Message)
		fmt.Println("================================================================================")
		return nil
	},
}

func init() {
	postCmd.Flags().StringVarP(&postSubject, "title", "t", "", "主贴标题")
	postCmd.Flags().StringVarP(&postMessage, "message", "m", "", "主贴正文内容")
	postCmd.Flags().StringVarP(&postFile, "file", "f", "", "从文本或 Markdown 文件中读取主贴正文")
	postCmd.Flags().BoolVar(&postForce, "force", false, "强制提交，跳过本地 30 秒防重检查")

	RootCmd.AddCommand(postCmd)
}
