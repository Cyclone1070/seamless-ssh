package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/seamless-ssh/sssh/cmd"
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
