package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

)

type CmdRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type Manager struct {
	runner CmdRunner
}

func NewManager(runner CmdRunner) *Manager {
	return &Manager{runner: runner}
}

func sessionName(localPath string) string {
	base := filepath.Base(localPath)
	var sb strings.Builder
	sb.WriteString("sssh-")
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	return sb.String()
}

func (m *Manager) Start(localPath string, sshTarget string, remotePath string) error {
	name := sessionName(localPath)
	remoteURL := fmt.Sprintf("%s:%s", sshTarget, remotePath)

	// Check if session already exists
	listBytes, err := m.runner.Run("mutagen", "sync", "list", name)
	if err == nil && len(listBytes) > 0 {
		listOutput := string(listBytes)
		if strings.Contains(listOutput, "Name: "+name) || strings.Contains(listOutput, "Identifier: ") {
			hasLocal := strings.Contains(listOutput, "URL: "+localPath) || strings.Contains(listOutput, "URL: "+filepath.Clean(localPath))
			hasRemote := strings.Contains(listOutput, "URL: "+remoteURL)

			if hasLocal && hasRemote {
				// Existing session matches targets, do nothing
				return nil
			}

			// Differing targets or duplicates exist, terminate them first
			_, _ = m.runner.Run("mutagen", "sync", "terminate", name)
		}
	}

	_, err = m.runner.Run("mutagen", "sync", "create", "--name", name, localPath, remoteURL)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "executable file not found") {
			return errors.New("mutagen is not installed locally")
		}
		if strings.Contains(errStr, "already exists") || strings.Contains(errStr, "already in use") {
			return nil
		}
		return err
	}
	return nil
}

func (m *Manager) Stop(localPath string) error {
	name := sessionName(localPath)
	_, err := m.runner.Run("mutagen", "sync", "terminate", name)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "session not found") || strings.Contains(errStr, "does not exist") {
			return nil
		}
		return err
	}
	return nil
}

func (m *Manager) WaitSync(localPath string, timeout time.Duration) error {
	name := sessionName(localPath)
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			_, _ = fmt.Fprintln(os.Stderr, "[sssh] Sync timeout, running command...")
			return nil
		}

		output, err := m.runner.Run("mutagen", "sync", "list", name)
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "mutagen daemon is not running") {
				// Try starting mutagen daemon
				_, _ = m.runner.Run("mutagen", "daemon", "start")
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return err
		}

		statusLine := ""
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Status:") {
				statusLine = trimmed
				break
			}
		}

		if statusLine == "" {
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if strings.Contains(statusLine, "Watching for changes") || strings.Contains(statusLine, "Idle") {
			return nil
		}

		if strings.Contains(statusLine, "Conflicts detected") || strings.Contains(statusLine, "Conflict:") {
			return errors.New("sync conflicts detected")
		}

		if strings.Contains(statusLine, "session not found") {
			return errors.New("session not found")
		}

		time.Sleep(50 * time.Millisecond)
	}
}
