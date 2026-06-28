package sync_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Cyclone1070/seamless-ssh/internal/sync"
)

type mockCmdRunner struct {
	runs       [][]string
	runResults map[string][]byte
	runErrors  map[string]error
	callCount  int
}

func (m *mockCmdRunner) Run(name string, args ...string) ([]byte, error) {
	m.callCount++
	cmdKey := name
	for _, arg := range args {
		cmdKey += " " + arg
	}

	// Capture execution
	m.runs = append(m.runs, append([]string{name}, args...))

	if err, ok := m.runErrors[cmdKey]; ok {
		return nil, err
	}
	if res, ok := m.runResults[cmdKey]; ok {
		return res, nil
	}

	// Default fallback matcher
	for k, v := range m.runResults {
		if reflect.DeepEqual(args, []string{k}) {
			return v, nil
		}
	}

	return nil, nil
}

func TestSyncStart_Success(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: make(map[string][]byte),
		runErrors:  make(map[string]error),
	}
	mgr := sync.NewManager(runner)

	host := "ubuntu@1.2.3.4:22"

	err := mgr.Start("/local/path", host, "/remote/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.runs) == 0 {
		t.Fatal("expected commands to be run, got none")
	}

	// Verify mutagen sync create was called
	createCmd := runner.runs[0]
	if createCmd[0] != "mutagen" || createCmd[1] != "sync" || createCmd[2] != "create" {
		t.Errorf("expected mutagen sync create, got: %v", createCmd)
	}
}

func TestSyncStart_AlreadyExists(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: make(map[string][]byte),
		runErrors: map[string]error{
			"mutagen sync create --name sssh-path /local/path ubuntu@1.2.3.4:22:/remote/path": errors.New("session already exists"),
		},
	}
	mgr := sync.NewManager(runner)

	host := "ubuntu@1.2.3.4:22"
	err := mgr.Start("/local/path", host, "/remote/path")
	if err != nil {
		// Should succeed or handle gracefully (returns nil if already syncs)
		t.Fatalf("expected already exists to be handled, got: %v", err)
	}
}

func TestSyncStart_MutagenBinaryNotFound(t *testing.T) {
	runner := &mockCmdRunner{
		runErrors: map[string]error{
			"mutagen sync create --name sssh-path /local/path ubuntu@1.2.3.4:22:/remote/path": errors.New("exec: \"mutagen\": executable file not found in $PATH"),
		},
	}
	mgr := sync.NewManager(runner)

	host := "ubuntu@1.2.3.4:22"
	err := mgr.Start("/local/path", host, "/remote/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "mutagen is not installed locally" {
		t.Errorf("expected mutagen is not installed error, got: %v", err)
	}
}

func TestSyncStart_ExitCodeError(t *testing.T) {
	runner := &mockCmdRunner{
		runErrors: map[string]error{
			"mutagen sync create --name sssh-path /local/path ubuntu@1.2.3.4:22:/remote/path": errors.New("exit status 1"),
		},
	}
	mgr := sync.NewManager(runner)

	host := "ubuntu@1.2.3.4:22"
	err := mgr.Start("/local/path", host, "/remote/path")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSyncStop_Success(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: make(map[string][]byte),
	}
	mgr := sync.NewManager(runner)

	err := mgr.Stop("/local/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	terminateCmd := runner.runs[0]
	if terminateCmd[0] != "mutagen" || terminateCmd[1] != "sync" || terminateCmd[2] != "terminate" {
		t.Errorf("expected mutagen sync terminate, got: %v", terminateCmd)
	}
}

func TestSyncStop_SessionNotFound(t *testing.T) {
	runner := &mockCmdRunner{
		runErrors: map[string]error{
			"mutagen sync terminate sssh-path": errors.New("exit status 1: error: session not found"),
		},
	}
	mgr := sync.NewManager(runner)

	err := mgr.Stop("/local/path")
	if err != nil {
		t.Fatalf("expected terminate for nonexistent session to be ignored, got: %v", err)
	}
}

func TestSyncStatus_ImmediateWatch(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: map[string][]byte{
			"mutagen sync list sssh-path": []byte("Status: Watching for changes\n"),
		},
	}
	mgr := sync.NewManager(runner)

	err := mgr.WaitSync("/local/path", 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncStatus_PollingCatchUp(t *testing.T) {
	// Status checks will fail/poll and then pass
	runner := &mockCmdRunner{
		runResults: make(map[string][]byte),
	}

	mgr := sync.NewManager(runner)

	// Since mockCmdRunner increments a call count or returns status dynamically:
	// We can write a custom function for runResults or evaluate callCount
	// In the implemented WaitSync, we call "mutagen sync list sssh-path" in a loop.
	// We will override runner's behavior to return Scanning -> Staging -> Watching on successive calls.
	var calls int
	runner.runResults["mutagen sync list sssh-path"] = []byte("Status: Scanning files\n") // initial

	// To mock progressive status, we can change the value of runResults during execution.
	// But in unit tests we can just mock a custom implementation of runner
	customRunner := &customMockRunner{
		states: []string{
			"Status: Scanning files\n",
			"Status: Staging files\n",
			"Status: Watching for changes\n",
		},
	}
	mgr = sync.NewManager(customRunner)

	err := mgr.WaitSync("/local/path", 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if customRunner.calls != 3 {
		t.Errorf("expected 3 status calls, got %d", calls)
	}
}

type customMockRunner struct {
	states []string
	calls  int
}

func (m *customMockRunner) Run(name string, args ...string) ([]byte, error) {
	state := m.states[m.calls]
	m.calls++
	return []byte(state), nil
}

func TestSyncStatus_Conflict(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: map[string][]byte{
			"mutagen sync list sssh-path": []byte("Status: Conflicts detected\nConflict: file modified on both sides"),
		},
	}
	mgr := sync.NewManager(runner)

	err := mgr.WaitSync("/local/path", 1*time.Second)
	if err == nil {
		t.Fatal("expected sync conflict error, got nil")
	}
	if err.Error() != "sync conflicts detected" {
		t.Errorf("expected conflict error message, got: %v", err)
	}
}

func TestSyncStatus_Timeout(t *testing.T) {
	// Polling stuck on Scanning files
	runner := &mockCmdRunner{
		runResults: map[string][]byte{
			"mutagen sync list sssh-path": []byte("Status: Scanning files\n"),
		},
	}
	mgr := sync.NewManager(runner)

	// Set a very small timeout for test
	err := mgr.WaitSync("/local/path", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("expected timeout to warn but return nil, got error: %v", err)
	}
}

func TestSyncStatus_DaemonNotRunning(t *testing.T) {
	// If list fails because mutagen daemon is not running, we try mutagen daemon start.
	customRunner := &daemonMockRunner{}
	mgr := sync.NewManager(customRunner)

	err := mgr.WaitSync("/local/path", 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !customRunner.daemonStarted {
		t.Fatal("expected mutagen daemon start to be executed")
	}
}

type daemonMockRunner struct {
	calls         int
	daemonStarted bool
}

func (m *daemonMockRunner) Run(name string, args ...string) ([]byte, error) {
	m.calls++
	cmd := name
	for _, arg := range args {
		cmd += " " + arg
	}

	if cmd == "mutagen sync list sssh-path" {
		if !m.daemonStarted {
			return nil, errors.New("exit status 1: error: mutagen daemon is not running")
		}
		return []byte("Status: Watching for changes\n"), nil
	}

	if cmd == "mutagen daemon start" {
		m.daemonStarted = true
		return nil, nil
	}

	return nil, nil
}
