package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Cyclone1070/sssh/cmd"
)

func TestNewRootCmd_Help(t *testing.T) {
	rootCmd := cmd.NewRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	expectedSubcommands := []string{"init", "link", "unlink", "add", "remove", "status", "check-intercept", "port-listener"}
	for _, sub := range expectedSubcommands {
		if !strings.Contains(out, sub) {
			t.Errorf("expected help output to contain subcommand %q, got: %s", sub, out)
		}
	}
}

func TestNewRootCmd_Version(t *testing.T) {
	// Verify --version flag output
	rootCmd := cmd.NewRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"--version"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sssh version") {
		t.Errorf("expected version output to contain 'sssh version', got: %q", out)
	}

	// Verify -v flag output
	rootCmd2 := cmd.NewRootCmd()
	buf2 := new(bytes.Buffer)
	rootCmd2.SetOut(buf2)
	rootCmd2.SetErr(buf2)
	rootCmd2.SetArgs([]string{"-v"})

	err = rootCmd2.Execute()
	if err != nil {
		t.Fatalf("unexpected error on -v: %v", err)
	}

	out2 := buf2.String()
	if !strings.Contains(out2, "sssh version") {
		t.Errorf("expected version output on -v to contain 'sssh version', got: %q", out2)
	}
}

