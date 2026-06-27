package ssh_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/seamless-ssh/sssh/internal/domain"
	"github.com/seamless-ssh/sssh/internal/ssh"
)

type mockCmdRunner struct {
	runs          [][]string
	runResults    map[string][]byte
	runErrors     map[string]error
	streamCode    int
	streamError   error
	streamInvoked bool
	streamArgs    []string
	streamEnv     []string
}

func (m *mockCmdRunner) Run(name string, args ...string) ([]byte, error) {
	cmdKey := name
	for _, arg := range args {
		cmdKey += " " + arg
	}
	m.runs = append(m.runs, append([]string{name}, args...))

	if err, ok := m.runErrors[cmdKey]; ok {
		return nil, err
	}
	if res, ok := m.runResults[cmdKey]; ok {
		return res, nil
	}
	return nil, nil
}

func (m *mockCmdRunner) RunStream(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	m.streamInvoked = true
	m.streamArgs = args
	m.streamEnv = env
	m.runs = append(m.runs, append([]string{name}, args...))

	if stdin != nil {
		_, _ = io.Copy(stdout, stdin) // Echo stdin to stdout for testing
	}

	return m.streamCode, m.streamError
}

func TestPathResolver_ExactMatch(t *testing.T) {
	resolver := ssh.NewResolver()
	link := domain.Link{
		LocalPath:  "/Users/mac/proj",
		RemotePath: "/remote/proj",
	}

	res, err := resolver.Resolve("/Users/mac/proj", link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "/remote/proj" {
		t.Errorf("expected /remote/proj, got %q", res)
	}
}

func TestPathResolver_NestedPath(t *testing.T) {
	resolver := ssh.NewResolver()
	link := domain.Link{
		LocalPath:  "/Users/mac/proj",
		RemotePath: "/remote/proj",
	}

	res, err := resolver.Resolve("/Users/mac/proj/src/utils", link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "/remote/proj/src/utils" {
		t.Errorf("expected /remote/proj/src/utils, got %q", res)
	}
}

func TestPathResolver_OutsideLink(t *testing.T) {
	resolver := ssh.NewResolver()
	link := domain.Link{
		LocalPath:  "/Users/mac/proj",
		RemotePath: "/remote/proj",
	}

	_, err := resolver.Resolve("/Users/mac/other", link)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "path is outside of linked directory hierarchy" {
		t.Errorf("expected outside hierarchy error, got: %v", err)
	}
}

func TestPathResolver_CaseSensitivityAndNormalization(t *testing.T) {
	resolver := ssh.NewResolver()
	link := domain.Link{
		LocalPath:  "/Users/mac/proj/",
		RemotePath: "/remote/proj/",
	}

	// Normalizing trailing slashes
	res, err := resolver.Resolve("/Users/mac/proj", link)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "/remote/proj" {
		t.Errorf("expected /remote/proj, got %q", res)
	}
}

func TestSSHExec_Success(t *testing.T) {
	runner := &mockCmdRunner{
		streamCode: 0,
	}
	mgr := ssh.NewManager(runner)

	host := "ubuntu@1.2.3.4"

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	stdin := bytes.NewReader([]byte("test input"))

	code, err := mgr.Exec(host, "/remote/dir", []string{"go", "test"}, []string{"VAR=VAL"}, stdin, stdout, stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}

	if !runner.streamInvoked {
		t.Fatal("expected RunStream to be called")
	}

	// Verify ControlMaster arguments
	args := runner.streamArgs
	if args[0] != "-o" || args[1] != "ControlMaster=auto" {
		t.Errorf("expected ControlMaster flags, got: %v", args)
	}

	// Stdin should have been copied to stdout in our mock
	if stdout.String() != "test input" {
		t.Errorf("expected stdout to receive stdin input 'test input', got: %q", stdout.String())
	}
}

func TestSSHExec_ExitCodes(t *testing.T) {
	runner := &mockCmdRunner{
		streamCode: 127,
	}
	mgr := ssh.NewManager(runner)

	host := "ubuntu@1.2.3.4"
	code, err := mgr.Exec(host, "/remote/dir", []string{"invalid"}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 127 {
		t.Errorf("expected exit code 127, got %d", code)
	}
}

func TestSSHExec_NetworkTimeout(t *testing.T) {
	runner := &mockCmdRunner{
		streamCode:  255,
		streamError: errors.New("ssh connection lost"),
	}
	mgr := ssh.NewManager(runner)

	host := "ubuntu@1.2.3.4"
	code, err := mgr.Exec(host, "/remote/dir", []string{"go", "test"}, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code != 255 {
		t.Errorf("expected exit code 255 on network failure, got %d", code)
	}
}
