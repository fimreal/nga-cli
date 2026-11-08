package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"nga/internal/filter"
)

var (
	readPage       int
	readAuthorOnly bool
	readFilterUIDs string
	readIncludeUID string
	readCompact    bool
	readRefresh    bool
)

var readCmd = &cobra.Command{
	Use:   "read <tid>",
	Short: "读取指定帖子的内容",
	Long:  "根据帖子 TID 读取对应页码的帖子内容，支持只看楼主、过滤指定 UID、以及紧凑输出。",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tid, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("无效的帖子 TID: %s", args[0])
		}

		cli, err := getClient()
		if err != nil {
			return fmt.Errorf("初始化客户端失败: %w", err)
		}

		ctx := context.Background()
		thread, err := cli.FetchThreadPage(ctx, tid, readPage, readRefresh)
		if err != nil {
			return fmt.Errorf("读取帖子失败: %w", err)
		}

		// Apply filters
		filterOpts := filter.Options{
			AuthorOnly:  readAuthorOnly,
			FilterUIDs:  parseUIDList(readFilterUIDs),
			IncludeUIDs: parseUIDList(readIncludeUID),
		}
		filtered := filter.Apply(thread, filterOpts)

		if flagJSON {
			return outputJSON(filtered)
		}

		PrintThread(filtered, readCompact)
		return nil
	},
}

func init() {
	readCmd.Flags().IntVarP(&readPage, "page", "p", 1, "指定读取的页码")
	readCmd.Flags().BoolVarP(&readAuthorOnly, "author", "a", false, "仅查看楼主发言")
	readCmd.Flags().StringVar(&readFilterUIDs, "filter-uid", "", "屏蔽/过滤的 UID 列表 (逗号分隔)")
	readCmd.Flags().StringVar(&readIncludeUID, "include-uid", "", "仅显示的 UID 列表 (逗号分隔)")
	readCmd.Flags().BoolVarP(&readCompact, "compact", "c", false, "紧凑单行格式输出")
	readCmd.Flags().BoolVarP(&readRefresh, "refresh", "r", false, "强制刷新，跳过本地缓存拉取最新数据")

	RootCmd.AddCommand(readCmd)
}
