package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"nga/pkg/model"
)

// RawResponse represents the root JSON structure of NGA lite=js response.
type RawResponse struct {
	Data  RawData     `json:"data"`
	Error interface{} `json:"error"`
	Msg   string      `json:"msg"`
}

type RawData struct {
	Global      map[string]interface{} `json:"__GLOBAL"`
	Users       map[string]RawUser     `json:"__U"`
	Rows        map[string]RawPost     `json:"__R"`
	Thread      RawThread              `json:"__T"`
	TotalRows   int                    `json:"__ROWS"`
	TotalRRows  int                    `json:"__R__ROWS"`
	RowsPerPage int                    `json:"__R__ROWS_PAGE"`
	Page        int                    `json:"__PAGE"`
}

type RawThread struct {
	Tid      int64       `json:"tid"`
	Fid      int64       `json:"fid"`
	Subject  string      `json:"subject"`
	Author   string      `json:"author"`
	AuthorID interface{} `json:"authorid"`
	Replies  int         `json:"replies"`
}

type RawUser struct {
	UID      interface{} `json:"uid"`
	Username string      `json:"username"`
}

type RawPost struct {
	Lou       int         `json:"lou"`
	Pid       int64       `json:"pid"`
	AuthorID  interface{} `json:"authorid"`
	PostDate  interface{} `json:"postdate"`
	Subject   string      `json:"subject"`
	Content   string      `json:"content"`
	AlterInfo string      `json:"alterinfo"`
}

// ParseLiteJS parses the raw JS/JSON response from NGA lite=js endpoint.
func ParseLiteJS(body []byte) (*model.ThreadDetail, error) {
	// 1. Locate the JSON payload
	start := bytes.IndexByte(body, '{')
	end := bytes.LastIndexByte(body, '}')
	if start == -1 || end == -1 || end <= start {
		// Check if it's a plain text error message
		text := strings.TrimSpace(string(body))
		if strings.Contains(text, "ERROR:") || strings.Contains(text, "访客不能直接访问") {
			return nil, fmt.Errorf("NGA WAF/Access error: %s", text)
		}
		return nil, fmt.Errorf("invalid NGA response: no JSON payload found")
	}

	rawJSON := body[start : end+1]

	// 2. Sanitize control characters inside JSON strings
	sanitized := sanitizeJSON(rawJSON)

	// 3. Unmarshal into raw response
	var resp RawResponse
	if err := json.Unmarshal(sanitized, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal NGA JSON: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("NGA error: %v", resp.Error)
	}

	data := resp.Data
	threadAuthorID := toInt64(data.Thread.AuthorID)

	// Calculate pagination metrics
	rowsPerPage := data.RowsPerPage
	if rowsPerPage <= 0 {
		rowsPerPage = 20
	}

	totalPosts := data.Thread.Replies + 1
	if data.TotalRRows > totalPosts {
		totalPosts = data.TotalRRows
	}
	if data.TotalRows > totalPosts {
		totalPosts = data.TotalRows
	}

	totalPages := 1
	if totalPosts > 0 {
		totalPages = (totalPosts + rowsPerPage - 1) / rowsPerPage
	}

	currentPage := data.Page
	if currentPage <= 0 {
		currentPage = 1
	}

	// 4. Transform posts
	posts := make([]model.Post, 0, len(data.Rows))
	for _, rawP := range data.Rows {
		authorID := toInt64(rawP.AuthorID)

		// Author name resolution
		authorName := ""
		if authorID == threadAuthorID && data.Thread.Author != "" {
			authorName = data.Thread.Author
		} else if user, exists := data.Users[strconv.FormatInt(authorID, 10)]; exists && user.Username != "" {
			authorName = user.Username
		}
		if authorName == "" {
			authorName = fmt.Sprintf("UID:%d", authorID)
		}

		postDateStr := formatPostDate(rawP.PostDate)

		p := model.Post{
			Floor:     rawP.Lou,
			Pid:       rawP.Pid,
			Tid:       data.Thread.Tid,
			AuthorID:  authorID,
			Author:    authorName,
			PostDate:  postDateStr,
			Subject:   rawP.Subject,
			Content:   CleanContent(rawP.Content),
			AlterInfo: rawP.AlterInfo,
		}
		posts = append(posts, p)
	}

	// Sort posts by floor ascending
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Floor < posts[j].Floor
	})

	threadTitle := data.Thread.Subject
	if threadTitle == "" && len(posts) > 0 && posts[0].Floor == 0 {
		threadTitle = posts[0].Subject
	}

	return &model.ThreadDetail{
		Tid:         data.Thread.Tid,
		Fid:         data.Thread.Fid,
		Title:       threadTitle,
		AuthorID:    threadAuthorID,
		Author:      data.Thread.Author,
		TotalPosts:  totalPosts,
		CurrentPage: currentPage,
		TotalPages:  totalPages,
		Posts:       posts,
	}, nil
}

// sanitizeJSON escapes raw unescaped control characters inside JSON strings.
func sanitizeJSON(src []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(src) + 64)
	inString := false
	escaped := false

	for i := 0; i < len(src); i++ {
		b := src[i]
		if inString {
			if escaped {
				buf.WriteByte(b)
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				buf.WriteByte(b)
				continue
			}
			if b == '"' {
				inString = false
				buf.WriteByte(b)
				continue
			}
			// Handle literal control characters inside string
			if b == '\r' {
				continue
			} else if b == '\n' {
				buf.WriteString(`\n`)
				continue
			} else if b == '\t' {
				buf.WriteString(`\t`)
				continue
			} else if b < 0x20 {
				buf.WriteString(fmt.Sprintf(`\u%04x`, b))
				continue
			}
			buf.WriteByte(b)
		} else {
			if b == '"' {
				inString = true
			}
			buf.WriteByte(b)
		}
	}
	return buf.Bytes()
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	case json.Number:
		n, _ := val.Int64()
		return n
	default:
		return 0
	}
}

func formatPostDate(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		t := time.Unix(int64(val), 0)
		return t.Format("2006-01-02 15:04:05")
	case int64:
		t := time.Unix(val, 0)
		return t.Format("2006-01-02 15:04:05")
	default:
		return ""
	}
}
