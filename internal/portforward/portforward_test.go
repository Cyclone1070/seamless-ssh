package portforward_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/seamless-ssh/sssh/internal/portforward"
)

type mockCmdRunner struct {
	runs          [][]string
	runResults    map[string][]byte
	runErrors     map[string]error
	streamStdout  string
	streamInvoked bool
	streamArgs    []string
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
	m.runs = append(m.runs, append([]string{name}, args...))

	if m.streamStdout != "" {
		_, _ = stdout.Write([]byte(m.streamStdout))
	}
	return 0, nil
}

type mockNetListener struct {
	addr        string
	closed      bool
	acceptError error
}

func (m *mockNetListener) Accept() (portforward.Conn, error) {
	if m.acceptError != nil {
		return nil, m.acceptError
	}
	return nil, errors.New("mock closed")
}

func (m *mockNetListener) Close() error {
	m.closed = true
	return nil
}

func (m *mockNetListener) Addr() portforward.Addr {
	return mockAddr{addr: m.addr}
}

type mockAddr struct {
	addr string
}

func (m mockAddr) Network() string { return "tcp" }
func (m mockAddr) String() string  { return m.addr }

type mockNetProvider struct {
	listeners   map[string]*mockNetListener
	listenError error
}

func (m *mockNetProvider) Listen(network, address string) (portforward.Listener, error) {
	if m.listenError != nil {
		return nil, m.listenError
	}
	if l, ok := m.listeners[address]; ok {
		return l, nil
	}
	l := &mockNetListener{addr: address}
	m.listeners[address] = l
	return l, nil
}

func TestPortParser_DockerSuccess(t *testing.T) {
	inspectOutput := []byte(`{
		"80/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}],
		"443/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8443"}]
	}`)
	parser := portforward.NewParser()
	ports, err := parser.ParseDocker(inspectOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"80/tcp":  "8080",
		"443/tcp": "8443",
	}
	if !reflectEqual(ports, expected) {
		t.Errorf("expected %+v, got %+v", expected, ports)
	}
}

func TestPortParser_DockerInvalidJSON(t *testing.T) {
	parser := portforward.NewParser()
	_, err := parser.ParseDocker([]byte(`invalid json`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPortParser_PodmanSuccess(t *testing.T) {
	inspectOutput := []byte(`[
		{
			"HostPort": 8080,
			"ContainerPort": 80,
			"Protocol": "tcp"
		}
	]`)
	parser := portforward.NewParser()
	ports, err := parser.ParsePodman(inspectOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"80/tcp": "8080",
	}
	if !reflectEqual(ports, expected) {
		t.Errorf("expected %+v, got %+v", expected, ports)
	}
}

func TestProxy_PortCollision(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: make(map[string][]byte),
	}
	customNetProv := &customNetProvider{
		failPorts: map[string]bool{
			"127.0.0.1:8080": true,
		},
		listeners: make(map[string]*mockNetListener),
	}

	mgr := portforward.NewManager(runner, customNetProv)
	host := "ubuntu@1.2.3.4"

	err := mgr.ProxyPort(host, "8080")
	if err == nil {
		t.Fatal("expected error on port collision, got nil")
	}
	expectedMsg := "Failed to forward port. Local port 8080 is in used."
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("expected error msg to contain %q, got: %v", expectedMsg, err)
	}
}

type customNetProvider struct {
	failPorts map[string]bool
	listeners map[string]*mockNetListener
}

func (c *customNetProvider) Listen(network, address string) (portforward.Listener, error) {
	if c.failPorts[address] {
		return nil, errors.New("port already bound")
	}
	l := &mockNetListener{addr: address}
	c.listeners[address] = l
	return l, nil
}

func TestProxy_TearDownOnStop(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: make(map[string][]byte),
	}
	netProv := &customNetProvider{
		listeners: make(map[string]*mockNetListener),
	}
	mgr := portforward.NewManager(runner, netProv)
	host := "ubuntu@1.2.3.4"

	err := mgr.ProxyPort(host, "9000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	l, ok := netProv.listeners["127.0.0.1:9000"]
	if !ok {
		t.Fatal("expected listener to exist")
	}

	mgr.StopAll()
	if !l.closed {
		t.Error("expected listener to be closed upon StopAll")
	}
}

func TestPortListener_EventStreamParsing(t *testing.T) {
	// Mock docker events stdout
	runner := &mockCmdRunner{
		streamStdout: "container-abc\ncontainer-def\n",
		runResults: map[string][]byte{
			"docker inspect --format {{json .NetworkSettings.Ports}} container-abc": []byte(`{"80/tcp": [{"HostPort": "8080"}]}`),
			"docker inspect --format {{json .NetworkSettings.Ports}} container-def": []byte(`{"3000/tcp": [{"HostPort": "3000"}]}`),
		},
	}
	netProv := &customNetProvider{
		listeners: make(map[string]*mockNetListener),
	}

	mgr := portforward.NewManager(runner, netProv)
	host := "ubuntu@1.2.3.4"

	// Run event listener in background
	go func() {
		_ = mgr.ListenEvents(host, false) // isPodman = false
	}()

	// Wait briefly for events to process
	time.Sleep(100 * time.Millisecond)

	// Verify both ports 8080 and 3000 got listeners
	if _, ok := netProv.listeners["127.0.0.1:8080"]; !ok {
		t.Error("expected port 8080 proxy to be active")
	}
	if _, ok := netProv.listeners["127.0.0.1:3000"]; !ok {
		t.Error("expected port 3000 proxy to be active")
	}
}

func reflectEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestProxy_ProxyWarningOutputsToStderr(t *testing.T) {
	runner := &mockCmdRunner{
		runResults: make(map[string][]byte),
	}
	// Mock collision
	netProv := &customNetProvider{
		failPorts: map[string]bool{
			"127.0.0.1:8080": true,
		},
		listeners: make(map[string]*mockNetListener),
	}

	mgr := portforward.NewManager(runner, netProv)
	host := "ubuntu@1.2.3.4"

	// Custom stderr capture
	oldStderr := portforward.StderrWriter
	defer func() { portforward.StderrWriter = oldStderr }()
	buf := new(bytes.Buffer)
	portforward.StderrWriter = buf

	_ = mgr.ProxyPort(host, "8080")

	output := buf.String()
	expectedWarning := "[sssh] Failed to forward port. Local port 8080 is in used.\n"
	if output != expectedWarning {
		t.Errorf("expected warning %q, got %q", expectedWarning, output)
	}
}
