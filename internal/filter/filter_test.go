package filter

import (
	"testing"

	"nga/pkg/model"
)

func TestFilter(t *testing.T) {
	thread := &model.ThreadDetail{
		Tid:      12345,
		AuthorID: 100,
		Posts: []model.Post{
			{Floor: 0, AuthorID: 100, Author: "楼主"},
			{Floor: 1, AuthorID: 200, Author: "用户A"},
			{Floor: 2, AuthorID: 300, Author: "用户B"},
			{Floor: 3, AuthorID: 100, Author: "楼主回复"},
			{Floor: 4, AuthorID: 400, Author: "用户C"},
		},
	}

	// Test 1: Author only
	t.Run("AuthorOnly", func(t *testing.T) {
		res := Apply(thread, Options{AuthorOnly: true})
		if len(res.Posts) != 2 {
			t.Fatalf("expected 2 posts, got %d", len(res.Posts))
		}
		for _, p := range res.Posts {
			if p.AuthorID != 100 {
				t.Errorf("expected author 100, got %d", p.AuthorID)
			}
		}
	})

	// Test 2: FilterUID (blacklist)
	t.Run("FilterUID", func(t *testing.T) {
		res := Apply(thread, Options{FilterUIDs: []int64{200, 400}})
		if len(res.Posts) != 3 {
			t.Fatalf("expected 3 posts, got %d", len(res.Posts))
		}
		for _, p := range res.Posts {
			if p.AuthorID == 200 || p.AuthorID == 400 {
				t.Errorf("blocked user %d found in posts", p.AuthorID)
			}
		}
	})

	// Test 3: IncludeUID (whitelist)
	t.Run("IncludeUID", func(t *testing.T) {
		res := Apply(thread, Options{IncludeUIDs: []int64{300, 400}})
		if len(res.Posts) != 2 {
			t.Fatalf("expected 2 posts, got %d", len(res.Posts))
		}
		if res.Posts[0].AuthorID != 300 || res.Posts[1].AuthorID != 400 {
			t.Errorf("unexpected posts in whitelist: %+v", res.Posts)
		}
	})
}
