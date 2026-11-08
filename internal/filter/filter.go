package filter

import (
	"nga/pkg/model"
)

// Options specifies filtering rules for posts in a thread.
type Options struct {
	AuthorOnly  bool
	FilterUIDs  []int64
	IncludeUIDs []int64
}

// Apply applies filter options to the thread posts and returns a new ThreadDetail.
func Apply(thread *model.ThreadDetail, opts Options) *model.ThreadDetail {
	if thread == nil {
		return nil
	}

	filterSet := make(map[int64]struct{}, len(opts.FilterUIDs))
	for _, uid := range opts.FilterUIDs {
		filterSet[uid] = struct{}{}
	}

	includeSet := make(map[int64]struct{}, len(opts.IncludeUIDs))
	for _, uid := range opts.IncludeUIDs {
		includeSet[uid] = struct{}{}
	}

	filteredPosts := make([]model.Post, 0, len(thread.Posts))
	for _, post := range thread.Posts {
		// 1. Author only check
		if opts.AuthorOnly && post.AuthorID != thread.AuthorID {
			continue
		}

		// 2. Filter UID check (blacklist)
		if _, blocked := filterSet[post.AuthorID]; blocked {
			continue
		}

		// 3. Include UID check (whitelist)
		if len(includeSet) > 0 {
			if _, included := includeSet[post.AuthorID]; !included {
				continue
			}
		}

		filteredPosts = append(filteredPosts, post)
	}

	// Create a shallow copy with filtered posts
	res := *thread
	res.Posts = filteredPosts
	return &res
}
