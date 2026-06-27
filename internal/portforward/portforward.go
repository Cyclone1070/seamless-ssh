package portforward

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

)

func controlPath(sshTarget string) string {
	var sb strings.Builder
	for _, r := range sshTarget {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	return filepath.Join("/tmp", fmt.Sprintf("sssh-%s", sb.String()))
}

var StderrWriter io.Writer = os.Stderr

type CmdRunner interface {
	Run(name string, args ...string) ([]byte, error)
	RunStream(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

type Addr interface {
	Network() string
	String() string
}

type Conn interface {
	io.ReadWriteCloser
}

type Listener interface {
	Accept() (Conn, error)
	Close() error
	Addr() Addr
}

type NetProvider interface {
	Listen(network, address string) (Listener, error)
}

type realAddr struct {
	a net.Addr
}

func (r realAddr) Network() string { return r.a.Network() }
func (r realAddr) String() string  { return r.a.String() }

type realListener struct {
	l net.Listener
}

func (r realListener) Accept() (Conn, error) {
	c, err := r.l.Accept()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r realListener) Close() error {
	return r.l.Close()
}

func (r realListener) Addr() Addr {
	return realAddr{a: r.l.Addr()}
}

type RealNetProvider struct{}

func (RealNetProvider) Listen(network, address string) (Listener, error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	return realListener{l: l}, nil
}

type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) ParseDocker(inspectOutput []byte) (map[string]string, error) {
	type dockerPort struct {
		HostPort string `json:"HostPort"`
	}
	var raw map[string][]dockerPort
	if err := json.Unmarshal(inspectOutput, &raw); err != nil {
		return nil, err
	}
	res := make(map[string]string)
	for k, v := range raw {
		if len(v) > 0 && v[0].HostPort != "" {
			res[k] = v[0].HostPort
		}
	}
	return res, nil
}

func (p *Parser) ParsePodman(inspectOutput []byte) (map[string]string, error) {
	type podmanPort struct {
		HostPort      int    `json:"HostPort"`
		ContainerPort int    `json:"ContainerPort"`
		Protocol      string `json:"Protocol"`
	}
	var raw []podmanPort
	if err := json.Unmarshal(inspectOutput, &raw); err != nil {
		// Podman inspect sometimes wraps the output in an array
		type podmanInspectWrapper struct {
			NetworkSettings struct {
				Ports []podmanPort `json:"Ports"`
			} `json:"NetworkSettings"`
		}
		var wrapper []podmanInspectWrapper
		if errArray := json.Unmarshal(inspectOutput, &wrapper); errArray == nil && len(wrapper) > 0 {
			raw = wrapper[0].NetworkSettings.Ports
		} else {
			return nil, err
		}
	}
	res := make(map[string]string)
	for _, port := range raw {
		key := fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol)
		res[key] = strconv.Itoa(port.HostPort)
	}
	return res, nil
}

type Manager struct {
	runner    CmdRunner
	netProv   NetProvider
	listeners []Listener
	mu        sync.Mutex
	quitChan  chan struct{}
}

func NewManager(runner CmdRunner, netProv NetProvider) *Manager {
	return &Manager{
		runner:   runner,
		netProv:  netProv,
		quitChan: make(chan struct{}),
	}
}

func (m *Manager) ProxyPort(sshTarget string, targetPort string) error {
	addr := "127.0.0.1:" + targetPort
	listener, err := m.netProv.Listen("tcp", addr)
	if err != nil {
		if StderrWriter != nil {
			_, _ = fmt.Fprintf(StderrWriter, "[sssh] Failed to forward port. Local port %s is in used.\n", targetPort)
		}
		return fmt.Errorf("Failed to forward port. Local port %s is in used.", targetPort)
	}

	m.mu.Lock()
	m.listeners = append(m.listeners, listener)
	m.mu.Unlock()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go m.handleConnection(sshTarget, conn, targetPort)
		}
	}()

	return nil
}

func (m *Manager) handleConnection(sshTarget string, conn Conn, targetPort string) {
	defer conn.Close()

	controlPath := controlPath(sshTarget)

	sshArgs := []string{
		"-o", "ControlPath=" + controlPath,
		sshTarget, "-W", "127.0.0.1:"+targetPort,
	}

	_, _ = m.runner.RunStream("ssh", sshArgs, nil, conn, conn, io.Discard)
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.listeners {
		if l != nil {
			_ = l.Close()
		}
	}
	m.listeners = nil
	select {
	case <-m.quitChan:
	default:
		close(m.quitChan)
	}
}

func (m *Manager) ListenEvents(sshTarget string, isPodman bool) error {
	pr, pw := io.Pipe()
	go func() {
		var cmdArgs []string
		binary := "docker"
		if isPodman {
			binary = "podman"
			cmdArgs = []string{"events", "--filter", "event=start"}
		} else {
			cmdArgs = []string{"events", "--filter", "event=start", "--format", "{{.ID}}"}
		}

		controlPath := controlPath(sshTarget)

		sshArgs := []string{
			"-o", "ControlPath=" + controlPath,
		}

		fullCmd := []string{binary}
		fullCmd = append(fullCmd, cmdArgs...)
		sshArgs = append(sshArgs, sshTarget, strings.Join(fullCmd, " "))

		_, _ = m.runner.RunStream("ssh", sshArgs, nil, nil, pw, io.Discard)
		_ = pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		containerID := strings.TrimSpace(scanner.Text())
		if containerID == "" {
			continue
		}
		go m.inspectAndProxy(sshTarget, containerID, isPodman)
	}
	return nil
}

func (m *Manager) inspectAndProxy(sshTarget string, containerID string, isPodman bool) {
	binary := "docker"
	format := "{{json .NetworkSettings.Ports}}"
	if isPodman {
		binary = "podman"
		format = "{{json .NetworkSettings.Ports}}" // Podman inspect behaves similarly or can be customized
	}

	output, err := m.runner.Run(binary, "inspect", "--format", format, containerID)
	if err != nil {
		return
	}

	parser := NewParser()
	var ports map[string]string
	if isPodman {
		ports, err = parser.ParsePodman(output)
	} else {
		ports, err = parser.ParseDocker(output)
	}

	if err != nil {
		return
	}

	for _, hostPort := range ports {
		_ = m.ProxyPort(sshTarget, hostPort)
	}
}
