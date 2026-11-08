package model

// Post represents a single reply/floor in an NGA thread.
type Post struct {
	Floor     int    `json:"floor"`
	Pid       int64  `json:"pid"`
	Tid       int64  `json:"tid"`
	AuthorID  int64  `json:"author_id"`
	Author    string `json:"author_name"`
	PostDate  string `json:"post_date"`
	Subject   string `json:"subject,omitempty"`
	Content   string `json:"content"`
	AlterInfo string `json:"alter_info,omitempty"`
}

// ThreadDetail represents a thread and its collection of posts.
type ThreadDetail struct {
	Tid         int64  `json:"tid"`
	Fid         int64  `json:"fid"`
	Title       string `json:"title"`
	AuthorID    int64  `json:"author_id"`
	Author      string `json:"author_name"`
	TotalPosts  int    `json:"total_posts"`
	CurrentPage int    `json:"current_page"`
	TotalPages  int    `json:"total_pages"`
	Posts       []Post `json:"posts"`
}

// EndpointCheck represents the result of DNS and ping checks for a domain.
type EndpointCheck struct {
	Domain    string   `json:"domain"`
	IPs       []string `json:"ips"`
	LatencyMs int64    `json:"latency_ms"`
	Error     string   `json:"error,omitempty"`
}

// DoctorReport represents the overall connectivity and WAF diagnostic report.
type DoctorReport struct {
	Timestamp      string          `json:"timestamp"`
	Endpoints      []EndpointCheck `json:"endpoints"`
	Proxy          string          `json:"proxy,omitempty"`
	EgressIP       string          `json:"egress_ip,omitempty"`
	ProbeSuccess   bool            `json:"probe_success"`
	ProbeHTTPCode  int             `json:"probe_http_code"`
	ProbeLatencyMs int64           `json:"probe_latency_ms"`
	WAFChallenged  bool            `json:"waf_challenged"`
	Message        string          `json:"message"`
}
