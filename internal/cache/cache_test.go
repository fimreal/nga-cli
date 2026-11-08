package cache

import (
	"os"
	"testing"

	"nga/pkg/model"
)

func TestCacheStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nga-cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("failed to init cache store: %v", err)
	}
	defer store.Close()

	thread := &model.ThreadDetail{
		Tid:         55555,
		Fid:         10,
		Title:       "缓存测试帖子",
		AuthorID:    1234,
		Author:      "测试员",
		TotalPosts:  45,
		CurrentPage: 1,
		TotalPages:  3,
		Posts: []model.Post{
			{Floor: 0, Pid: 1000, Tid: 55555, AuthorID: 1234, Author: "测试员", Content: "首楼内容"},
			{Floor: 1, Pid: 1001, Tid: 55555, AuthorID: 5678, Author: "回复者", Content: "一楼回复"},
		},
	}

	// 1. Initially cache should miss
	_, hit := store.GetPage(55555, 1, false)
	if hit {
		t.Errorf("expected cache miss initially")
	}

	// 2. Save page 1
	if err := store.SavePage(thread, 1); err != nil {
		t.Fatalf("failed to save page: %v", err)
	}

	// 3. Cache should hit
	cached, hit := store.GetPage(55555, 1, false)
	if !hit {
		t.Fatalf("expected cache hit after save")
	}

	if cached.Title != "缓存测试帖子" {
		t.Errorf("expected title '缓存测试帖子', got %q", cached.Title)
	}
	if len(cached.Posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(cached.Posts))
	}
	if cached.Posts[0].Content != "首楼内容" {
		t.Errorf("expected content '首楼内容', got %q", cached.Posts[0].Content)
	}

	// 4. Test bypass cache
	_, hit = store.GetPage(55555, 1, true)
	if hit {
		t.Errorf("expected bypassCache to result in miss")
	}

	// 5. Test clear cache
	if err := store.Clear(55555); err != nil {
		t.Fatalf("failed to clear cache: %v", err)
	}

	_, hit = store.GetPage(55555, 1, false)
	if hit {
		t.Errorf("expected miss after clear")
	}
}

func TestCacheHistoricalVsActivePage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nga-cache-policy-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := New(tempDir)
	if err != nil {
		t.Fatalf("failed to init cache store: %v", err)
	}
	defer store.Close()

	// Simulate 20 posts for historical page
	var fullPosts []model.Post
	for i := 0; i < 20; i++ {
		fullPosts = append(fullPosts, model.Post{Floor: i, Content: "test"})
	}

	thread := &model.ThreadDetail{
		Tid:        999,
		TotalPages: 5,
		Posts:      fullPosts,
	}

	// Save page 1 (historical page < totalPages)
	if err := store.SavePage(thread, 1); err != nil {
		t.Fatalf("failed to save historical page: %v", err)
	}

	// Check historical page cached
	cached, hit := store.GetPage(999, 1, false)
	if !hit || len(cached.Posts) != 20 {
		t.Errorf("expected historical page hit")
	}
}
