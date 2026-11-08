package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"nga/internal/client"
	"nga/pkg/model"
)

var (
	replyMessage string
	replyFile    string
	replyQuote   int
	replyFid     int64
	replyForce   bool
)

var replyCmd = &cobra.Command{
	Use:   "reply <tid>",
	Short: "对指定的 NGA 帖子发表回复（支持引用指定楼层与防重保护）",
	Long:  "向目标 TID 发表新回复，支持直接传参或从文件读取正文；提供 --quote 自动提取被引用楼层组装规范 BBCode，并内置 30 秒防重保护。",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("无效的帖子 TID: %s", args[0])
		}

		var content string
		if replyFile != "" {
			data, err := os.ReadFile(replyFile)
			if err != nil {
				return fmt.Errorf("读取回复内容文件失败: %w", err)
			}
			content = string(data)
		} else if replyMessage != "" {
			content = replyMessage
		} else {
			return fmt.Errorf("请通过 -m/--message 或 -f/--file 提供回复内容")
		}

		content = strings.TrimSpace(content)
		if content == "" {
			return fmt.Errorf("回复内容不能为空")
		}

		cli, err := getClient()
		if err != nil {
			return fmt.Errorf("初始化客户端失败: %w", err)
		}

		ctx := context.Background()

		// If quote is requested, fetch the target floor
		var quotePost *model.Post
		if cmd.Flags().Changed("quote") {
			targetPage := (replyQuote / 20) + 1
			thread, err := cli.FetchThreadPage(ctx, tid, targetPage, false)
			if err != nil {
				return fmt.Errorf("获取被引用楼层失败: %w", err)
			}
			for _, p := range thread.Posts {
				if p.Floor == replyQuote {
					qp := p
					quotePost = &qp
					break
				}
			}
			if quotePost == nil {
				return fmt.Errorf("在 TID %d 中未找到 #%d 楼内容", tid, replyQuote)
			}
		}

		opts := client.ReplyOptions{
			Tid:       tid,
			Fid:       replyFid,
			Content:   content,
			QuotePost: quotePost,
			Force:     replyForce,
		}

		res, err := cli.PostReply(ctx, opts)
		if err != nil {
			return err
		}

		if flagJSON {
			return outputJSON(res)
		}

		fmt.Println("================================================================================")
		fmt.Printf("✅ 回复成功！TID: %d\n", tid)
		if quotePost != nil {
			fmt.Printf("已引用楼层: #%d 楼 (%s)\n", quotePost.Floor, quotePost.Author)
		}
		fmt.Printf("响应结果: %s\n", res.Message)
		fmt.Println("================================================================================")
		return nil
	},
}

func init() {
	replyCmd.Flags().StringVarP(&replyMessage, "message", "m", "", "回复内容文本")
	replyCmd.Flags().StringVarP(&replyFile, "file", "f", "", "从文本或 Markdown 文件中读取回复内容")
	replyCmd.Flags().IntVar(&replyQuote, "quote", -1, "引用指定楼层 (例如 --quote 0 引用主贴)")
	replyCmd.Flags().Int64Var(&replyFid, "fid", 0, "帖子所属板块 FID (可选)")
	replyCmd.Flags().BoolVar(&replyForce, "force", false, "强制提交，跳过本地 30 秒防重检查")

	RootCmd.AddCommand(replyCmd)
}
