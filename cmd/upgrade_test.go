package cmd

import (
	"bytes"
	"testing"
)

func TestUpgradeCommandRegistered(t *testing.T) {
	cmd, _, err := RootCmd.Find([]string{"upgrade"})
	if err != nil {
		t.Fatalf("upgrade command not registered: %v", err)
	}

	for _, flagName := range []string{"check", "version", "force", "no-checksum", "repo", "api-base", "bin"} {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("missing --%s flag on upgrade", flagName)
		}
	}

	for _, alias := range []string{"update", "self-update"} {
		found := false
		for _, a := range cmd.Aliases {
			if a == alias {
				found = true
			}
		}
		if !found {
			t.Errorf("missing alias %q", alias)
		}
	}
}

func TestUpgradeHelpRenders(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"upgrade", "--help"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("GitHub Release")) {
		t.Errorf("help output missing release source: %s", buf.String())
	}
}

func TestFirstConfigured(t *testing.T) {
	if got := firstConfigured("", "  ", " fimreal/nga-cli "); got != "fimreal/nga-cli" {
		t.Errorf("firstConfigured = %q, want the trimmed value", got)
	}
	if got := firstConfigured("", ""); got != "" {
		t.Errorf("firstConfigured = %q, want empty", got)
	}
}
