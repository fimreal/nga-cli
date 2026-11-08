package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestCheckInHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	checkInCmd.SetOut(buf)
	checkInCmd.SetErr(buf)
	err := checkInCmd.Help()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "每日签到") {
		t.Errorf("expected help output to mention '每日签到', got: %s", output)
	}
	if !strings.Contains(output, "--force") {
		t.Errorf("expected help output to have '--force', got: %s", output)
	}
	if !strings.Contains(output, "--stat") {
		t.Errorf("expected help output to have '--stat', got: %s", output)
	}
}

func TestCheckInUnauthenticated(t *testing.T) {
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"checkin", "--profile", "nonexistent"})

	err := RootCmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unauthenticated checkin, got nil")
	}
	if !strings.Contains(err.Error(), "未鉴权") {
		t.Errorf("expected error message to mention '未鉴权', got: %v", err)
	}
}
