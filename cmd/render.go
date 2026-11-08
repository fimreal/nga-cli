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
