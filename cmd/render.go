package cmd

import (
	"fmt"
	"strings"

	"nga/pkg/model"
)

// PrintThread prints the thread details and posts to stdout.
func PrintThread(thread *model.ThreadDetail, compact bool) {
	if thread == nil {
		fmt.Println("无帖子数据。")
		return
	}

	divider := strings.Repeat("=", 80)

	fmt.Println(divider)
	fmt.Printf("[TID: %d] %s\n", thread.Tid, thread.Title)
	fmt.Printf("作者: %s (UID: %d) | 总楼层: %d | 当前页: %d/%d\n",
		thread.Author, thread.AuthorID, thread.TotalPosts, thread.CurrentPage, thread.TotalPages)
	fmt.Println(divider)

	if len(thread.Posts) == 0 {
		fmt.Println("（没有符合过滤条件的楼层）")
		return
	}

	for _, post := range thread.Posts {
		PrintPost(post, compact)
	}
}

// PrintPost formats and prints a single post to stdout.
func PrintPost(post model.Post, compact bool) {
	subDivider := strings.Repeat("-", 80)
	if compact {
		preview := strings.ReplaceAll(post.Content, "\n", " ")
		if len([]rune(preview)) > 80 {
			preview = string([]rune(preview)[:80]) + "..."
		}
		fmt.Printf("[#%d] %s | %s(UID:%d) | %s\n",
			post.Floor, post.PostDate, post.Author, post.AuthorID, preview)
	} else {
		fmt.Println(subDivider)
		fmt.Printf("#%d 楼 | %s (UID: %d) | %s\n",
			post.Floor, post.Author, post.AuthorID, post.PostDate)
		fmt.Println(subDivider)
		fmt.Println(post.Content)
		fmt.Println()
	}
}

// PrintDoctorReport prints the connectivity and WAF diagnostic report.
func PrintDoctorReport(report *model.DoctorReport) {
	divider := strings.Repeat("=", 80)
	fmt.Println(divider)
	fmt.Println("NGA CLI Doctor - 网络连通性与反风控诊断报告")
	fmt.Printf("检测时间: %s\n", report.Timestamp)
	proxyStr := report.Proxy
	if proxyStr == "" {
		proxyStr = "直连（未配置代理）"
	}
	fmt.Printf("代理配置: %s\n", proxyStr)
	fmt.Println(divider)

	fmt.Println("1. 域名 DNS 解析测试:")
	for _, ep := range report.Endpoints {
		if ep.Error != "" {
			fmt.Printf("   - %-15s [FAIL] 解析失败: %s\n", ep.Domain, ep.Error)
		} else {
			ipList := strings.Join(ep.IPs, ", ")
			fmt.Printf("   - %-15s [OK] IP: [%s] (耗时: %dms)\n", ep.Domain, ipList, ep.LatencyMs)
		}
	}

	fmt.Println()
	fmt.Println("2. 核心端点探测 (lite=js + 移动端 UA):")
	fmt.Printf("   - HTTP 状态码: %d\n", report.ProbeHTTPCode)
	fmt.Printf("   - 请求延时   : %dms\n", report.ProbeLatencyMs)
	if report.WAFChallenged {
		fmt.Println("   - WAF/拦截   : [WARN] 遭遇 WAF 拦截或频率限制")
	} else if report.ProbeSuccess {
		fmt.Println("   - WAF/拦截   : [PASS] 成功绕过 WAF，获取到合法结构化 JSON")
	} else {
		fmt.Println("   - WAF/拦截   : [FAIL] 响应异常")
	}

	fmt.Println()
	fmt.Printf("诊断结论: %s\n", report.Message)
	fmt.Println(divider)
}

// PrintUpgradeResult renders a self-update check or installation as a card.
func PrintUpgradeResult(res *model.UpgradeResult) {
	divider := strings.Repeat("=", 80)
	fmt.Println(divider)
	if res.Applied {
		fmt.Println("✅ NGA CLI 自更新完成")
	} else if res.UpdateAvailable {
		fmt.Println("⬆️  NGA CLI 发现可用更新")
	} else {
		fmt.Println("ℹ️  NGA CLI 版本检查")
	}
	fmt.Printf("当前版本: %s\n", res.CurrentVersion)
	fmt.Printf("目标版本: %s\n", res.TargetVersion)
	if res.ReleaseName != "" {
		fmt.Printf("发布名称: %s\n", res.ReleaseName)
	}
	if res.PublishedAt != "" {
		fmt.Printf("发布时间: %s\n", res.PublishedAt)
	}
	if res.AssetName != "" {
		fmt.Printf("平台资产: %s (%s)\n", res.AssetName, humanBytes(res.AssetSize))
	}
	if res.TargetPath != "" {
		fmt.Printf("目标路径: %s\n", res.TargetPath)
	}
	if res.ChecksumVerified {
		fmt.Printf("完整性  : SHA256 %s (已校验)\n", res.Checksum)
	}
	fmt.Printf("结果: %s\n", res.Message)
	if res.ReleaseURL != "" {
		fmt.Printf("发行页: %s\n", res.ReleaseURL)
	}
	fmt.Println(divider)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
