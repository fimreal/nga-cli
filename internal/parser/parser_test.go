package parser

import (
	"testing"
)

func TestCleanContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "br to newline",
			input:    "line1<br/>line2<br>line3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "html entities",
			input:    "&lt;test&gt; &amp; &quot;quote&quot;",
			expected: "<test> & \"quote\"",
		},
		{
			name:     "quote bbcode",
			input:    "[quote][pid=123,456,1]Reply[/pid] [b]Post by [uid=999]user[/uid]:[/b]<br/>quoted text[/quote]my reply",
			expected: "> Post by user:\n> quoted text\n\nmy reply",
		},
		{
			name:     "images",
			input:    "see picture: [img]./mon_202401/03/test.jpg[/img]",
			expected: "see picture: [图片: ./mon_202401/03/test.jpg]",
		},
		{
			name:     "url",
			input:    "[url=https://bbs.nga.cn]NGA论坛[/url]",
			expected: "NGA论坛 (https://bbs.nga.cn)",
		},
		{
			name:     "bold and strike",
			input:    "[b]bold text[/b] and [del]deleted text[/del]",
			expected: "bold text and ~~deleted text~~",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanContent(tt.input)
			if got != tt.expected {
				t.Errorf("CleanContent(%q) =\n%q, want\n%q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseLiteJS(t *testing.T) {
	raw := `window.script_muti_get_var_store={"data":{"__GLOBAL":{"_ATTACH_BASE_VIEW":"img.nga.cn"},"__U":{"1001":{"uid":1001,"username":"楼主甲"},"1002":{"uid":1002,"username":"回复乙"}},"__R":{"0":{"lou":0,"pid":111,"authorid":1001,"postdate":"2026-11-08 10:00","subject":"主贴标题","content":"这是主楼内容\n包含未转义换行<br/>第二行"},"1":{"lou":1,"pid":112,"authorid":1002,"postdate":"2026-11-08 10:05","subject":"","content":"这是1楼回复[b]加粗[/b]"}},"__T":{"tid":99999,"fid":10,"subject":"主贴标题","author":"楼主甲","authorid":1001,"replies":1},"__ROWS":2,"__R__ROWS":2,"__R__ROWS_PAGE":20,"__PAGE":1}};`

	thread, err := ParseLiteJS([]byte(raw))
	if err != nil {
		t.Fatalf("ParseLiteJS failed: %v", err)
	}

	if thread.Tid != 99999 {
		t.Errorf("expected tid 99999, got %d", thread.Tid)
	}
	if thread.Title != "主贴标题" {
		t.Errorf("expected title '主贴标题', got %q", thread.Title)
	}
	if thread.Author != "楼主甲" {
		t.Errorf("expected author '楼主甲', got %q", thread.Author)
	}
	if thread.TotalPosts != 2 {
		t.Errorf("expected 2 total posts, got %d", thread.TotalPosts)
	}
	if len(thread.Posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(thread.Posts))
	}

	if thread.Posts[0].Floor != 0 || thread.Posts[0].Author != "楼主甲" {
		t.Errorf("post 0 floor/author mismatch: %+v", thread.Posts[0])
	}
	if thread.Posts[1].Floor != 1 || thread.Posts[1].Author != "回复乙" {
		t.Errorf("post 1 floor/author mismatch: %+v", thread.Posts[1])
	}
	if thread.Posts[1].Content != "这是1楼回复加粗" {
		t.Errorf("expected post 1 content '这是1楼回复加粗', got %q", thread.Posts[1].Content)
	}
}
