package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nga/pkg/model"
)

func TestClientFetchThreadPage(t *testing.T) {
	// Mock server that returns a GBK encoded lite=js response
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != DefaultUserAgent {
			t.Errorf("unexpected User-Agent: %s", r.Header.Get("User-Agent"))
		}
		raw := `window.script_muti_get_var_store={"data":{"__GLOBAL":{},"__U":{"100":{"uid":100,"username":"MockUser"}},"__R":{"0":{"lou":0,"pid":1,"authorid":100,"postdate":"2026-11-08 12:00","subject":"Mock Title","content":"Hello from mock"}},"__T":{"tid":123,"fid":1,"subject":"Mock Title","author":"MockUser","authorid":100,"replies":0},"__ROWS":1,"__R__ROWS":1,"__R__ROWS_PAGE":20,"__PAGE":1}};`
		gbkBytes, _ := EncodeGBK(raw)
		w.Header().Set("Content-Type", "text/javascript; charset=GBK")
		w.Write(gbkBytes)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cli, err := New(Config{
		BaseURL:     server.URL,
		RateLimitMs: 10,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	thread, err := cli.FetchThreadPage(context.Background(), 123, 1, false)
	if err != nil {
		t.Fatalf("FetchThreadPage failed: %v", err)
	}

	if thread.Tid != 123 {
		t.Errorf("expected tid 123, got %d", thread.Tid)
	}
	if thread.Title != "Mock Title" {
		t.Errorf("expected title 'Mock Title', got %q", thread.Title)
	}
	if len(thread.Posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(thread.Posts))
	}
	if thread.Posts[0].Content != "Hello from mock" {
		t.Errorf("expected content 'Hello from mock', got %q", thread.Posts[0].Content)
	}
}

func TestClientDoctorProbe(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := `window.script_muti_get_var_store={"data":{"__T":{"tid":38888888}}};`
		w.Write([]byte(raw))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	cli, err := New(Config{
		BaseURL:     server.URL,
		RateLimitMs: 10,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	report, err := cli.DoctorProbe(context.Background(), 38888888)
	if err != nil {
		t.Fatalf("DoctorProbe failed: %v", err)
	}

	if !report.ProbeSuccess {
		fmt.Printf("report: %+v\n", report)
		t.Errorf("expected ProbeSuccess true, got false. Message: %s", report.Message)
	}
	if report.ProbeHTTPCode != 200 {
		t.Errorf("expected 200, got %d", report.ProbeHTTPCode)
	}
}


func TestClientFollowThread(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		// First call returns 1 post, subsequent calls return 2 posts
		var raw string
		if callCount == 1 {
			raw = `window.script_muti_get_var_store={"data":{"__GLOBAL":{},"__U":{"100":{"uid":100,"username":"MockUser"}},"__R":{"0":{"lou":0,"pid":1,"authorid":100,"postdate":"2026-11-08 12:00","subject":"Mock","content":"Floor 0"}},"__T":{"tid":123,"fid":1,"subject":"Mock","author":"MockUser","authorid":100,"replies":0},"__ROWS":1,"__R__ROWS":1,"__R__ROWS_PAGE":20,"__PAGE":1}};`
		} else {
			raw = `window.script_muti_get_var_store={"data":{"__GLOBAL":{},"__U":{"100":{"uid":100,"username":"MockUser"},"101":{"uid":101,"username":"NewUser"}},"__R":{"0":{"lou":0,"pid":1,"authorid":100,"postdate":"2026-11-08 12:00","subject":"Mock","content":"Floor 0"},"1":{"lou":1,"pid":2,"authorid":101,"postdate":"2026-11-08 12:05","subject":"","content":"Floor 1"}},"__T":{"tid":123,"fid":1,"subject":"Mock","author":"MockUser","authorid":100,"replies":1},"__ROWS":2,"__R__ROWS":2,"__R__ROWS_PAGE":20,"__PAGE":1}};`
		}
		gbkBytes, _ := EncodeGBK(raw)
		w.Header().Set("Content-Type", "text/javascript; charset=GBK")
		w.Write(gbkBytes)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cli, err := New(Config{
		BaseURL:     server.URL,
		RateLimitMs: 10,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var receivedFloors []int
	opts := FollowOptions{
		Tid:          123,
		InitialLines: 5,
		BaseInterval: 50 * time.Millisecond,
		MaxInterval:  200 * time.Millisecond,
		OnPost: func(post model.Post) {
			receivedFloors = append(receivedFloors, post.Floor)
			if post.Floor == 1 {
				cancel() // Stop following once floor 1 arrives
			}
		},
	}

	_ = cli.FollowThread(ctx, opts)

	if len(receivedFloors) < 2 {
		t.Fatalf("expected to receive at least 2 floors, got %v", receivedFloors)
	}
	if receivedFloors[0] != 0 || receivedFloors[1] != 1 {
		t.Errorf("expected floors [0, 1], got %v", receivedFloors)
	}
}


func TestClientPostReplyAndDuplicate(t *testing.T) {
	postedBody := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		postedBody = string(body)
		raw := `window.script_muti_get_var_store={"data":{"__MESSAGE":{"0":"操作成功","1":"发帖成功"}}};`
		gbkBytes, _ := EncodeGBK(raw)
		w.Write(gbkBytes)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	cli, err := New(Config{
		BaseURL:     server.URL,
		CookieUID:   1001,
		CookieCID:   "test_token",
		RateLimitMs: 10,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	quotePost := &model.Post{
		Pid:      555,
		Tid:      123,
		Floor:    2,
		AuthorID: 2002,
		Author:   "被引用人",
		PostDate: "2026-11-08 10:00:00",
		Content:  "引用的前文内容",
	}

	opts := ReplyOptions{
		Tid:       123,
		Content:   "这是我的测试回复",
		QuotePost: quotePost,
	}

	// 1. First reply should succeed
	res, err := cli.PostReply(context.Background(), opts)
	if err != nil {
		t.Fatalf("PostReply failed: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success true, got false")
	}

	if !strings.Contains(postedBody, "action=reply") || !strings.Contains(postedBody, "tid=123") {
		t.Errorf("unexpected form body: %s", postedBody)
	}

	// 2. Second reply immediately after should trigger anti-duplicate
	_, err = cli.PostReply(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "防重机制触发") {
		t.Errorf("expected duplicate error, got: %v", err)
	}

	// 3. With Force=true, it should bypass anti-duplicate
	opts.Force = true
	res2, err := cli.PostReply(context.Background(), opts)
	if err != nil || !res2.Success {
		t.Errorf("expected success with force=true, got err: %v", err)
	}
}

func TestClientPostUnauthenticated(t *testing.T) {
	cli, _ := New(Config{
		CookieUID: 0,
		CookieCID: "",
	})
	_, err := cli.PostReply(context.Background(), ReplyOptions{
		Tid:     123,
		Content: "test",
	})
	if err == nil || !strings.Contains(err.Error(), "未鉴权") {
		t.Errorf("expected unauthenticated error, got: %v", err)
	}
}

func TestClientCheckIn_Success(t *testing.T) {
	requestedPath := ""
	requestedUA := ""
	requestedXUA := ""
	requestedCookie := ""

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.String()
		requestedUA = r.Header.Get("User-Agent")
		requestedXUA = r.Header.Get("X-User-Agent")
		requestedCookie = r.Header.Get("Cookie")

		raw := `{"data":{"0":"每日签到成功，获得金币奖励"},"time":1794137000}`
		gbkBytes, _ := EncodeGBK(raw)
		w.Header().Set("Content-Type", "application/json; charset=GBK")
		w.Write(gbkBytes)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cli, err := New(Config{
		BaseURL:     server.URL,
		CookieUID:   88888,
		CookieCID:   "test_cid",
		RateLimitMs: 10,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	res, err := cli.CheckIn(context.Background(), false)
	if err != nil {
		t.Fatalf("CheckIn failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success true, got false")
	}
	if !strings.Contains(res.Message, "每日签到成功") {
		t.Errorf("unexpected message: %s", res.Message)
	}
	if !strings.Contains(requestedPath, "__lib=check_in&__act=check_in") {
		t.Errorf("unexpected requested path: %s", requestedPath)
	}
	if requestedXUA != "Nga_Official" {
		t.Errorf("expected X-User-Agent: Nga_Official, got: %s", requestedXUA)
	}
	if !strings.Contains(requestedCookie, "ngaPassportUid=88888") || !strings.Contains(requestedCookie, "ngaPassportCid=test_cid") {
		t.Errorf("unexpected cookie: %s", requestedCookie)
	}
	_ = requestedUA
}

func TestClientCheckIn_AlreadyDone(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := `{"error":{"0":"你今天已经签到过了"},"time":1794137000}`
		gbkBytes, _ := EncodeGBK(raw)
		w.Header().Set("Content-Type", "application/json; charset=GBK")
		w.Write(gbkBytes)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cli, err := New(Config{
		BaseURL:     server.URL,
		CookieUID:   88888,
		CookieCID:   "test_cid",
		RateLimitMs: 10,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	res, err := cli.CheckIn(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckIn with already done should not error, got: %v", err)
	}

	if !res.AlreadyDone {
		t.Errorf("expected AlreadyDone true, got false")
	}
	if !strings.Contains(res.Message, "已经签到") {
		t.Errorf("expected message containing '已经签到', got: %s", res.Message)
	}
}

func TestClientCheckIn_Unauthenticated(t *testing.T) {
	cli, _ := New(Config{
		CookieUID: 0,
		CookieCID: "",
	})
	_, err := cli.CheckIn(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "未鉴权") {
		t.Errorf("expected unauthenticated error, got: %v", err)
	}
}

func TestClientGetCheckInStat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := `{"data":{"continued":7,"sum":120},"time":1794137000}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(raw))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cli, err := New(Config{
		BaseURL:     server.URL,
		CookieUID:   88888,
		CookieCID:   "test_cid",
		RateLimitMs: 10,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	stat, err := cli.GetCheckInStat(context.Background())
	if err != nil {
		t.Fatalf("GetCheckInStat failed: %v", err)
	}
	if !stat.Success {
		t.Errorf("expected stat success true, got false")
	}
	if !strings.Contains(stat.Stat, "continued") {
		t.Errorf("expected stat containing 'continued', got: %s", stat.Stat)
	}
}
