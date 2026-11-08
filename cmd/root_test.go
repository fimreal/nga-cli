package cmd

import (
	"bytes"
	"testing"
)

func TestParseUIDList(t *testing.T) {
	uids := parseUIDList("1001, 1002,invalid,1003")
	if len(uids) != 3 {
		t.Fatalf("expected 3 valid uids, got %d", len(uids))
	}
	if uids[0] != 1001 || uids[1] != 1002 || uids[2] != 1003 {
		t.Errorf("unexpected uid values: %v", uids)
	}

	empty := parseUIDList("")
	if len(empty) != 0 {
		t.Errorf("expected empty slice for empty string, got %v", empty)
	}
}

func TestRootHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--help"})

	err := RootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Errorf("expected help output, got empty")
	}
}
