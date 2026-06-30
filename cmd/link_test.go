package cmd

import (
	"os"
	"testing"
)

type mockCmdRunner struct {
	runs [][]string
}

func (m *mockCmdRunner) Run(name string, args ...string) ([]byte, error) {
	m.runs = append(m.runs, append([]string{name}, args...))
	return nil, nil
}

func TestLink_CreatesRemoteDir(t *testing.T) {
	// Set up temporary home directory to avoid modifying user's links.json
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Mock command runner
	mockRunner := &mockCmdRunner{}
	oldRunner := commandRunner
	commandRunner = mockRunner
	defer func() {
		commandRunner = oldRunner
	}()

	// Save original args and restore later
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()

	// Set args to simulate: sssh link . dev-box --remote-dir /home/testuser/remote-dir
	os.Args = []string{"sssh", "link", ".", "dev-box", "--remote-dir", "/home/testuser/remote-dir"}

	// Run link
	runLink()

	// Verify that the remote directory creation command was executed via ssh
	// We expect command line like: ssh -o ControlPath=... dev-box mkdir -p /home/testuser/remote-dir
	mkdirCommandFound := false
	for _, run := range mockRunner.runs {
		if run[0] == "ssh" {
			// Find if "mkdir" and "-p" and "/home/testuser/remote-dir" are present in args
			hasMkdir := false
			hasP := false
			hasPath := false
			for _, arg := range run {
				if arg == "mkdir" {
					hasMkdir = true
				}
				if arg == "-p" {
					hasP = true
				}
				if arg == "/home/testuser/remote-dir" {
					hasPath = true
				}
			}
			if hasMkdir && hasP && hasPath {
				mkdirCommandFound = true
				break
			}
		}
	}

	if !mkdirCommandFound {
		t.Errorf("expected remote directory creation command to be executed, but got: %v", mockRunner.runs)
	}
}
