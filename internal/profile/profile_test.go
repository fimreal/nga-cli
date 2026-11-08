package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nga-profile-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfgPath := filepath.Join(tempDir, "profiles.yaml")
	mgr, err := NewManager(cfgPath)
	if err != nil {
		t.Fatalf("failed to create profile manager: %v", err)
	}

	// 1. Add profiles
	p1 := Profile{
		Name:      "default",
		CookieUID: 10001,
		CookieCID: "cid_111",
		Proxy:     "",
	}
	if err := mgr.Set(p1); err != nil {
		t.Fatalf("failed to set profile: %v", err)
	}

	p2 := Profile{
		Name:      "alt",
		CookieUID: 10002,
		CookieCID: "cid_222",
		Proxy:     "http://127.0.0.1:7890",
	}
	if err := mgr.Set(p2); err != nil {
		t.Fatalf("failed to set second profile: %v", err)
	}

	// 2. List profiles
	list, active := mgr.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(list))
	}
	if active != "default" {
		t.Errorf("expected active profile 'default', got %q", active)
	}

	// 3. Switch active profile
	if err := mgr.Use("alt"); err != nil {
		t.Fatalf("failed to switch profile: %v", err)
	}

	got, ok := mgr.Get("")
	if !ok || got.Name != "alt" || got.CookieUID != 10002 {
		t.Errorf("expected active profile 'alt' with uid 10002, got %+v", got)
	}

	// 4. Reload manager from disk to verify persistence
	mgr2, err := NewManager(cfgPath)
	if err != nil {
		t.Fatalf("failed to reload manager: %v", err)
	}
	got2, ok2 := mgr2.Get("alt")
	if !ok2 || got2.CookieUID != 10002 || got2.Proxy != "http://127.0.0.1:7890" {
		t.Errorf("persistence verification failed: %+v", got2)
	}

	// 5. Remove profile
	if err := mgr2.Remove("default"); err != nil {
		t.Fatalf("failed to remove profile: %v", err)
	}
	list2, _ := mgr2.List()
	if len(list2) != 1 || list2[0].Name != "alt" {
		t.Errorf("expected only 1 profile 'alt' remaining, got %v", list2)
	}
}
