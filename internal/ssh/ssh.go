package ssh

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/seamless-ssh/sssh/internal/domain"
)

type CmdRunner interface {
	Run(name string, args ...string) ([]byte, error)
	RunStream(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

type Resolver struct{}

func NewResolver() *Resolver {
	return &Resolver{}
}

func (r *Resolver) Resolve(localPath string, link domain.Link) (string, error) {
	localClean := filepath.Clean(localPath)
	linkClean := filepath.Clean(link.LocalPath)

	if localClean == linkClean {
		return filepath.ToSlash(filepath.Clean(link.RemotePath)), nil
	}

	prefix := linkClean + string(filepath.Separator)
	if !strings.HasPrefix(localClean, prefix) {
		return "", errors.New("path is outside of linked directory hierarchy")
	}

	rel, err := filepath.Rel(linkClean, localClean)
	if err != nil {
		return "", err
	}

	relRemote := filepath.ToSlash(rel)
	remoteClean := filepath.ToSlash(filepath.Clean(link.RemotePath))

	return strings.TrimSuffix(remoteClean, "/") + "/" + strings.TrimPrefix(relRemote, "/"), nil
}

type Manager struct {
	runner CmdRunner
}

func NewManager(runner CmdRunner) *Manager {
	return &Manager{runner: runner}
}

func escapeArg(arg string) string {
	if strings.ContainsAny(arg, " \t\n&|;<>()$`\"'\\*?[]~") {
		return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return arg
}

func (m *Manager) Exec(host domain.HostConfig, remoteDir string, cmdAndArgs []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	var envStrings []string
	for _, e := range env {
		if strings.Contains(e, "=") {
			parts := strings.SplitN(e, "=", 2)
			envStrings = append(envStrings, fmt.Sprintf("%s=%s", parts[0], escapeArg(parts[1])))
		}
	}

	var cmdParts []string
	for _, arg := range cmdAndArgs {
		cmdParts = append(cmdParts, escapeArg(arg))
	}

	var remoteCmd string
	if len(envStrings) > 0 {
		remoteCmd = fmt.Sprintf("cd %s && env %s %s", escapeArg(remoteDir), strings.Join(envStrings, " "), strings.Join(cmdParts, " "))
	} else {
		remoteCmd = fmt.Sprintf("cd %s && %s", escapeArg(remoteDir), strings.Join(cmdParts, " "))
	}

	portStr := strconv.Itoa(host.Port)
	if host.Port <= 0 {
		portStr = "22"
	}

	// We default to a standard temp dir or local folder for ssh ControlPath to avoid missing HOME env issues in tests.
	controlPath := filepath.Join("/tmp", fmt.Sprintf("sssh-%s-%s-%s", host.User, host.Host, portStr))

	sshArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlPath,
		"-o", "ControlPersist=1h",
		"-p", portStr,
	}

	if host.SSHKeyPath != "" {
		sshArgs = append(sshArgs, "-i", host.SSHKeyPath)
	}

	sshArgs = append(sshArgs, host.User+"@"+host.Host, remoteCmd)

	return m.runner.RunStream("ssh", sshArgs, nil, stdin, stdout, stderr)
}
