package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"nga/pkg/model"
)

const (
	HistoricalPageTTL = 24 * time.Hour
	ActivePageTTL     = 30 * time.Second
)

// Store handles local SQLite caching of NGA threads and pages.
type Store struct {
	db  *sql.DB
	mu  sync.RWMutex
	dir string
}

// New initializes an SQLite cache store at the given directory.
func New(cacheDir string) (*Store, error) {
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		cacheDir = filepath.Join(home, ".cache", "nga")
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dbPath := filepath.Join(cacheDir, "cache.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Optimize SQLite performance for concurrent read/write
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	_, _ = db.Exec("PRAGMA synchronous=NORMAL;")

	s := &Store{
		db:  db,
		dir: cacheDir,
	}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS threads (
		tid INTEGER PRIMARY KEY,
		fid INTEGER,
		title TEXT,
		author_id INTEGER,
		author TEXT,
		total_posts INTEGER,
		total_pages INTEGER,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS pages (
		tid INTEGER,
		page INTEGER,
		is_full INTEGER,
		cached_at INTEGER,
		posts_json TEXT,
		PRIMARY KEY (tid, page)
	);

	CREATE TABLE IF NOT EXISTS submissions (
		hash_key TEXT PRIMARY KEY,
		submitted_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS checkins (
		uid INTEGER,
		checkin_date TEXT,
		created_at INTEGER,
		PRIMARY KEY (uid, checkin_date)
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// GetPage retrieves a cached page of a thread if valid.
func (s *Store) GetPage(tid int64, page int, bypassCache bool) (*model.ThreadDetail, bool) {
	if bypassCache || s == nil || s.db == nil {
		return nil, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Query thread metadata
	var thread model.ThreadDetail
	var updatedAt int64
	err := s.db.QueryRow(`
		SELECT tid, fid, title, author_id, author, total_posts, total_pages, updated_at
		FROM threads WHERE tid = ?
	`, tid).Scan(
		&thread.Tid, &thread.Fid, &thread.Title,
		&thread.AuthorID, &thread.Author,
		&thread.TotalPosts, &thread.TotalPages, &updatedAt,
	)
	if err != nil {
		return nil, false
	}

	// 2. Query page posts
	var isFullInt int
	var cachedAt int64
	var postsJSON string
	err = s.db.QueryRow(`
		SELECT is_full, cached_at, posts_json
		FROM pages WHERE tid = ? AND page = ?
	`, tid, page).Scan(&isFullInt, &cachedAt, &postsJSON)
	if err != nil {
		return nil, false
	}

	isFull := isFullInt == 1
	age := time.Since(time.Unix(cachedAt, 0))

	// If page is full and before the last page, it's immutable historical data (valid for 24h)
	// Otherwise, it's the active last page with a short TTL (30s)
	if isFull && page < thread.TotalPages {
		if age > HistoricalPageTTL {
			return nil, false
		}
	} else {
		if age > ActivePageTTL {
			return nil, false
		}
	}

	var posts []model.Post
	if err := json.Unmarshal([]byte(postsJSON), &posts); err != nil {
		return nil, false
	}

	thread.CurrentPage = page
	thread.Posts = posts
	return &thread, true
}

// SavePage stores or updates thread metadata and page posts into the cache.
func (s *Store) SavePage(thread *model.ThreadDetail, page int) error {
	if s == nil || s.db == nil || thread == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	// 1. Upsert thread metadata
	_, err := s.db.Exec(`
		INSERT INTO threads (tid, fid, title, author_id, author, total_posts, total_pages, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tid) DO UPDATE SET
			fid = excluded.fid,
			title = excluded.title,
			author_id = excluded.author_id,
			author = excluded.author,
			total_posts = excluded.total_posts,
			total_pages = excluded.total_pages,
			updated_at = excluded.updated_at
	`, thread.Tid, thread.Fid, thread.Title, thread.AuthorID, thread.Author,
		thread.TotalPosts, thread.TotalPages, now)
	if err != nil {
		return err
	}

	// 2. Upsert page posts
	postsData, err := json.Marshal(thread.Posts)
	if err != nil {
		return err
	}

	isFull := 0
	if len(thread.Posts) >= 20 && page < thread.TotalPages {
		isFull = 1
	}

	_, err = s.db.Exec(`
		INSERT INTO pages (tid, page, is_full, cached_at, posts_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(tid, page) DO UPDATE SET
			is_full = excluded.is_full,
			cached_at = excluded.cached_at,
			posts_json = excluded.posts_json
	`, thread.Tid, page, isFull, now, string(postsData))

	return err
}

// Clear clears the cache for a given TID, or all if tid == 0.
func (s *Store) Clear(tid int64) error {
	if s == nil || s.db == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if tid == 0 {
		_, err1 := s.db.Exec("DELETE FROM threads")
		_, err2 := s.db.Exec("DELETE FROM pages")
		if err1 != nil {
			return err1
		}
		return err2
	}

	_, err1 := s.db.Exec("DELETE FROM threads WHERE tid = ?", tid)
	_, err2 := s.db.Exec("DELETE FROM pages WHERE tid = ?", tid)
	if err1 != nil {
		return err1
	}
	return err2
}

// Close closes the underlying SQLite database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}


// CheckAndRecordSubmission checks if a submission hash was recorded within the given TTL,
// and records the current timestamp if allowed.
func (s *Store) CheckAndRecordSubmission(hashKey string, ttl time.Duration, force bool) error {
	if s == nil || s.db == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	if !force {
		var submittedAt int64
		err := s.db.QueryRow("SELECT submitted_at FROM submissions WHERE hash_key = ?", hashKey).Scan(&submittedAt)
		if err == nil {
			elapsed := time.Since(time.Unix(submittedAt, 0))
			if elapsed < ttl {
				remain := int((ttl - elapsed).Seconds())
				if remain <= 0 {
					remain = 1
				}
				return fmt.Errorf("防重机制触发：检测到 %d 秒内向相同目标重复提交相同内容（冷却剩余 %d 秒）。如确需重发请使用 --force 参数", int(ttl.Seconds()), remain)
			}
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO submissions (hash_key, submitted_at)
		VALUES (?, ?)
		ON CONFLICT(hash_key) DO UPDATE SET submitted_at = excluded.submitted_at
	`, hashKey, now)
	return err
}

// HasCheckedInToday checks if the user has a recorded check-in for the given date.
func (s *Store) HasCheckedInToday(uid int64, dateStr string) bool {
	if s == nil || s.db == nil || uid <= 0 {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRow("SELECT COUNT(1) FROM checkins WHERE uid = ? AND checkin_date = ?", uid, dateStr).Scan(&count)
	return err == nil && count > 0
}

// RecordCheckIn records a successful or confirmed check-in for the user on dateStr.
func (s *Store) RecordCheckIn(uid int64, dateStr string) error {
	if s == nil || s.db == nil || uid <= 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO checkins (uid, checkin_date, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(uid, checkin_date) DO UPDATE SET created_at = excluded.created_at
	`, uid, dateStr, now)
	return err
}
