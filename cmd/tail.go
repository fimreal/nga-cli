package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"nga/internal/client"
	"nga/internal/filter"
	"nga/pkg/model"
)

var (
	tailLines      int
	tailAuthorOnly bool
	tailFilterUIDs string
	tailIncludeUID string
	tailCompact    bool
	tailFollow     bool
	tailInterval   time.Duration
)

var tailCmd = &cobra.Command{
	Use:   "tail <tid>",
	Short: "查看帖子末尾最新楼层 (支持 -f 流式持续监听与退避抖动)",
	Long:  "根据帖子总楼层直算末页页码，仅按需拉取末尾楼层对应的数据包；支持 -f/--follow 模式动态流式监听新楼层。",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("无效的帖子 TID: %s", args[0])
		}

		if tailLines <= 0 {
			tailLines = 10
		}

		cli, err := getClient()
		if err != nil {
			return fmt.Errorf("初始化客户端失败: %w", err)
		}

		// Apply filters
		filterOpts := filter.Options{
			AuthorOnly:  tailAuthorOnly,
			FilterUIDs:  parseUIDList(tailFilterUIDs),
			IncludeUIDs: parseUIDList(tailIncludeUID),
		}

		// Follow mode (tail -f)
		if tailFollow {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			fmt.Printf("开始实时监听 [TID: %d] 最新回复 (按 Ctrl+C 退出，基准轮询: %s，内置退避抖动)...\n\n", tid, tailInterval)

			followOpts := client.FollowOptions{
				Tid:           tid,
				InitialLines:  tailLines,
				BaseInterval:  tailInterval,
				MaxInterval:   10 * time.Second,
				IdleThreshold: 5,
				FilterOpts:    filterOpts,
				OnPost: func(post model.Post) {
					PrintPost(post, tailCompact)
				},
				OnError: func(err error) {
					fmt.Fprintf(os.Stderr, "[警告] 轮询出错: %v (已自动退避至安全等待)\n", err)
				},
			}

			if err := cli.FollowThread(ctx, followOpts); err != nil && err != context.Canceled {
				return err
			}
			fmt.Println("\n已停止监听。")
			return nil
		}

		// Standard tail
		ctx := context.Background()
		thread, err := cli.FetchThreadTail(ctx, tid, tailLines)
		if err != nil {
			return fmt.Errorf("读取帖子尾部失败: %w", err)
		}

		filtered := filter.Apply(thread, filterOpts)

		if flagJSON {
			return outputJSON(filtered)
		}

		PrintThread(filtered, tailCompact)
		return nil
	},
}

func init() {
	tailCmd.Flags().IntVarP(&tailLines, "lines", "n", 10, "查看最后 N 楼 (默认 10 楼)")
	tailCmd.Flags().BoolVarP(&tailAuthorOnly, "author", "a", false, "仅查看楼主发言")
	tailCmd.Flags().StringVar(&tailFilterUIDs, "filter-uid", "", "屏蔽/过滤的 UID 列表 (逗号分隔)")
	tailCmd.Flags().StringVar(&tailIncludeUID, "include-uid", "", "仅显示的 UID 列表 (逗号分隔)")
	tailCmd.Flags().BoolVarP(&tailCompact, "compact", "c", false, "紧凑单行格式输出")
	tailCmd.Flags().BoolVarP(&tailFollow, "follow", "f", false, "流式持续监听新回复 (类似 tail -f，带退避抖动)")
	tailCmd.Flags().DurationVar(&tailInterval, "interval", 3*time.Second, "轮询基准间隔 (默认 3s，内置 ±500ms 随机抖动)")

	RootCmd.AddCommand(tailCmd)
}
