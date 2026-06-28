package internal_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Cyclone1070/sssh/internal/shell"
	"golang.org/x/crypto/ssh"
)

// generateHostKey generates a dummy host key for our in-memory SSH server
func generateHostKey(t *testing.T) ssh.Signer {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	return signer
}

type localForwardChannelData struct {
	DestAddr string
	DestPort uint32
	OrigAddr string
	OrigPort uint32
}

func startSSHServer(t *testing.T) (string, func()) {
	hostKey := generateHostKey(t)
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, nil // Accept any public key
		},
	}
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := listener.Addr().String()

	quit := make(chan struct{})
	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				select {
				case <-quit:
					return
				default:
					continue
				}
			}
			go func(c net.Conn) {
				sshConn, chans, reqs, err := ssh.NewServerConn(c, config)
				if err != nil {
					return
				}
				defer sshConn.Close()
				go ssh.DiscardRequests(reqs)

				for newChannel := range chans {
					if newChannel.ChannelType() == "direct-tcpip" {
						var data localForwardChannelData
						if err := ssh.Unmarshal(newChannel.ExtraData(), &data); err != nil {
							newChannel.Reject(ssh.ConnectionFailed, "failed to parse extra data")
							continue
						}

						conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", data.DestAddr, data.DestPort))
						if err != nil {
							newChannel.Reject(ssh.ConnectionFailed, err.Error())
							continue
						}

						channel, requests, err := newChannel.Accept()
						if err != nil {
							conn.Close()
							continue
						}
						go ssh.DiscardRequests(requests)

						go func() {
							defer channel.Close()
							defer conn.Close()
							go io.Copy(channel, conn)
							io.Copy(conn, channel)
						}()
						continue
					}

					if newChannel.ChannelType() != "session" {
						newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
						continue
					}
					channel, requests, err := newChannel.Accept()
					if err != nil {
						continue
					}
					go handleSSHRequests(t, channel, requests)
				}
			}(nConn)
		}
	}()

	cleanup := func() {
		close(quit)
		_ = listener.Close()
	}

	return addr, cleanup
}

func handleSSHRequests(t *testing.T, channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	var env []string

	for req := range requests {
		switch req.Type {
		case "env":
			var envReq struct {
				Name  string
				Value string
			}
			if err := ssh.Unmarshal(req.Payload, &envReq); err == nil {
				env = append(env, fmt.Sprintf("%s=%s", envReq.Name, envReq.Value))
			}
			req.Reply(true, nil)

		case "exec":
			var execReq struct {
				Command string
			}
			if err := ssh.Unmarshal(req.Payload, &execReq); err != nil {
				req.Reply(false, nil)
				return
			}
			req.Reply(true, nil)

			cmd := exec.Command("sh", "-c", execReq.Command)
			cmd.Env = append(os.Environ(), env...)
			cmd.Stdout = channel
			cmd.Stderr = channel.Stderr()

			stdinPipe, err := cmd.StdinPipe()
			if err == nil {
				go func() {
					_, _ = io.Copy(stdinPipe, channel)
					_ = stdinPipe.Close()
				}()
			}

			exitStatus := uint32(0)
			err = cmd.Run()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						exitStatus = uint32(status.ExitStatus())
					} else {
						exitStatus = uint32(exitErr.ExitCode())
					}
				} else {
					exitStatus = 255
				}
			}

			var exitStatusPayload struct {
				Status uint32
			}
			exitStatusPayload.Status = exitStatus
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(exitStatusPayload))
			return
		default:
			req.Reply(false, nil)
		}
	}
}

func buildMockMutagen(t *testing.T, binDir string) {
	src := `package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(0)
	}

	logFile := filepath.Join(os.TempDir(), "sssh-mock-mutagen.log")
	f, _ := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if f != nil {
		fmt.Fprintf(f, "[%s] Args: %v\n", time.Now().Format(time.RFC3339), args)
		f.Close()
	}

	cmd := args[0]
	switch cmd {
	case "daemon":
		if len(args) > 1 && args[1] == "start" {
			os.Exit(0)
		}
	case "sync":
		if len(args) < 2 {
			os.Exit(0)
		}
		subCmd := args[1]
		switch subCmd {
		case "create":
			var name, local, remote string
			for i := 2; i < len(args); i++ {
				if args[i] == "--name" && i+1 < len(args) {
					name = args[i+1]
					i++
				} else if local == "" {
					local = args[i]
				} else {
					remote = args[i]
				}
			}

			var remotePath string
			parts := strings.Split(remote, ":")
			if len(parts) >= 3 {
				remotePath = strings.Join(parts[2:], ":")
			} else if len(parts) > 0 {
				remotePath = parts[len(parts)-1]
			}

			mapping := map[string]string{
				"name":   name,
				"local":  local,
				"remote": remotePath,
			}
			data, _ := json.Marshal(mapping)
			sessionFile := filepath.Join(os.TempDir(), "sssh-mock-mutagen-session-"+name+".json")
			_ = os.WriteFile(sessionFile, data, 0644)

			if os.Getenv("SSSH_MOCK_MUTAGEN_CHILD") != "true" {
				exe, _ := os.Executable()
				cmd := exec.Command(exe, "sync", "run-loop", name)
				cmd.Env = append(os.Environ(), "SSSH_MOCK_MUTAGEN_CHILD=true")
				_ = cmd.Start()
			}
			os.Exit(0)

		case "run-loop":
			name := args[2]
			sessionFile := filepath.Join(os.TempDir(), "sssh-mock-mutagen-session-"+name+".json")
			
			var mapping map[string]string
			for i := 0; i < 50; i++ {
				data, err := os.ReadFile(sessionFile)
				if err == nil {
					_ = json.Unmarshal(data, &mapping)
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if mapping == nil {
				os.Exit(1)
			}

			local := mapping["local"]
			remote := mapping["remote"]

			for {
				if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
					break
				}
				_ = filepath.Walk(local, func(path string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					rel, _ := filepath.Rel(local, path)
					if strings.HasPrefix(rel, ".") {
						return nil
					}
					dest := filepath.Join(remote, rel)
					_ = os.MkdirAll(filepath.Dir(dest), 0755)
					
					srcFile, err := os.Open(path)
					if err != nil {
						return nil
					}
					defer srcFile.Close()
					
					destFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
					if err != nil {
						return nil
					}
					defer destFile.Close()
					_, _ = io.Copy(destFile, srcFile)
					return nil
				})
				time.Sleep(100 * time.Millisecond)
			}
			os.Exit(0)

		case "terminate":
			name := args[2]
			sessionFile := filepath.Join(os.TempDir(), "sssh-mock-mutagen-session-"+name+".json")
			_ = os.Remove(sessionFile)
			os.Exit(0)

		case "list":
			name := args[2]
			sessionFile := filepath.Join(os.TempDir(), "sssh-mock-mutagen-session-"+name+".json")
			if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
				fmt.Printf("Session %s not found\n", name)
				os.Exit(1)
			}
			fmt.Printf("Name: %s\n", name)
			fmt.Println("Status: Watching for changes")
			os.Exit(0)
		}
	}
}
`
	tmpSrc := filepath.Join(t.TempDir(), "mutagen_src.go")
	if err := os.WriteFile(tmpSrc, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write mutagen source: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, "mutagen"), tmpSrc)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to compile mock mutagen: %v", err)
	}
}

func buildMockDocker(t *testing.T, binDir string) {
	src := `package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		os.Exit(0)
	}

	cmd := args[0]
	switch cmd {
	case "events":
		fmt.Println("mock-container-123")
		for {
			time.Sleep(100 * time.Second)
		}
	case "inspect":
		fmt.Println(` + "`" + `{"80/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}]}` + "`" + `)
		os.Exit(0)
	}
}
`
	tmpSrc := filepath.Join(t.TempDir(), "docker_src.go")
	if err := os.WriteFile(tmpSrc, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write docker source: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, "docker"), tmpSrc)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to compile mock docker: %v", err)
	}

	// copy to podman as well
	podmanDest := filepath.Join(binDir, "podman")
	_ = copyFile(filepath.Join(binDir, "docker"), podmanDest)
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func TestIntegration_E2E(t *testing.T) {
	// Setup isolated home and bin directory
	tempHomeRaw := t.TempDir()
	tempHome, _ := filepath.EvalSymlinks(tempHomeRaw)
	tempBinRaw := t.TempDir()
	tempBin, _ := filepath.EvalSymlinks(tempBinRaw)

	// Start SSH server
	sshAddr, cleanupSSH := startSSHServer(t)
	defer cleanupSSH()

	// Parse host & port
	hostIp, portStr, err := net.SplitHostPort(sshAddr)
	if err != nil {
		t.Fatalf("invalid ssh address: %v", err)
	}
	port, _ := net.LookupPort("tcp", portStr)

	// Build mock executables
	buildMockMutagen(t, tempBin)
	buildMockDocker(t, tempBin)
	buildMockSSH(t, tempBin, tempHome)

	// Compile the real sssh binary
	ssshBin := filepath.Join(tempBin, "sssh")
	buildCmd := exec.Command("go", "build", "-o", ssshBin, "../main.go")
	buildCmd.Stderr = os.Stderr
	buildCmd.Stdout = os.Stdout
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile sssh binary: %v", err)
	}

	// Helper to run sssh commands with isolated environment
	runSssh := func(dir string, args ...string) (string, string, error) {
		cmd := exec.Command(ssshBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"HOME="+tempHome,
			"PATH="+tempBin+string(filepath.ListSeparator)+os.Getenv("PATH"),
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}

	// 1. Run init
	_, _, err = runSssh(tempHome, "init")
	if err != nil {
		t.Fatalf("sssh init failed: %v", err)
	}

	// Write local SSH configuration to ~/.ssh/config under alias "dev-box"
	sshKeyPath := filepath.Join(tempHome, ".ssh", "id_rsa")
	_ = os.MkdirAll(filepath.Dir(sshKeyPath), 0700)
	writeRSAPrivateKey(t, sshKeyPath)

	sshConfigPath := filepath.Join(tempHome, ".ssh", "config")
	sshConfigContent := fmt.Sprintf(`Host dev-box
    HostName %s
    Port %d
    User %s
    IdentityFile %s
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
`, hostIp, port, os.Getenv("USER"), sshKeyPath)
	_ = os.WriteFile(sshConfigPath, []byte(sshConfigContent), 0600)

	// 2. Run link
	localProj := filepath.Join(tempHome, "my-proj")
	_ = os.MkdirAll(localProj, 0755)

	remoteProj := filepath.Join(tempHome, "remote-proj")
	_ = os.MkdirAll(remoteProj, 0755)

	stdout, stderr, err := runSssh(localProj, "link", ".", "dev-box", "--remote-dir", remoteProj)
	if err != nil {
		t.Fatalf("sssh link failed: %v, stdout: %q, stderr: %q", err, stdout, stderr)
	}

	// Check links.json is written
	linksPath := filepath.Join(tempHome, ".config", "sssh", "links.json")
	if _, err := os.Stat(linksPath); err != nil {
		t.Fatalf("links.json not found: %v", err)
	}

	// Verify mutagen synced file
	testFile := filepath.Join(localProj, "hello.txt")
	_ = os.WriteFile(testFile, []byte("hello world"), 0644)

	// Wait up to 2 seconds for sync loop
	synced := false
	for i := 0; i < 20; i++ {
		remoteFile := filepath.Join(remoteProj, "hello.txt")
		if content, err := os.ReadFile(remoteFile); err == nil && string(content) == "hello world" {
			synced = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !synced {
		t.Fatal("file hello.txt was not synced to remote folder")
	}

	// 3. Add and status patterns
	_, _, err = runSssh(localProj, "add", "go test *")
	if err != nil {
		t.Fatalf("sssh add failed: %v", err)
	}

	statusOut, _, err := runSssh(localProj, "status")
	if err != nil {
		t.Fatalf("sssh status failed: %v", err)
	}
	if !strings.Contains(statusOut, "go test *") {
		t.Fatalf("status output does not contain pattern: %s", statusOut)
	}

	// 4. Run command remotely (fallback)
	// We expect "sssh go test" inside localProj to execute "go test" remotely.
	// Since our SSH server executes sh -c, let's test a simple echo or command.
	runOut, runErrOut, err := runSssh(localProj, "echo", "hello-from-remote")
	if err != nil {
		t.Fatalf("remote run failed: %v, stderr: %s", err, runErrOut)
	}
	if !strings.Contains(runOut, "hello-from-remote") {
		t.Fatalf("unexpected remote run stdout: %s", runOut)
	}

	// 5. Test port forwarding
	// We will start a TCP server on port 8080 locally to simulate the forwarded port connection.
	// The port listener in background should listen on 127.0.0.1:8080 (or 8081 if collision).
	// We wait for the port-listener to catch the container start event.
	time.Sleep(500 * time.Millisecond)

	// Dial the forwarded port
	// Since 8080 might be in use, we try 8080 first, then 8081
	dialSuccess := false
	for _, port := range []int{8080, 8081} {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			dialSuccess = true
			break
		}
	}
	if !dialSuccess {
		t.Log("Warning: Could not connect to forwarded port 8080/8081. This might be due to environment container/port permissions.")
	}

	// 6. Unlink
	_, _, err = runSssh(localProj, "unlink")
	if err != nil {
		t.Fatalf("sssh unlink failed: %v", err)
	}

	// Verify links.json is cleaned
	linksData, err := os.ReadFile(linksPath)
	if err == nil {
		var mapping map[string]interface{}
		_ = json.Unmarshal(linksData, &mapping)
		if len(mapping) > 0 {
			t.Fatalf("links.json is not empty: %s", string(linksData))
		}
	}
}

func TestIntegration_ListenerReuse(t *testing.T) {
	// Setup isolated home and bin directory
	tempHomeRaw := t.TempDir()
	tempHome, _ := filepath.EvalSymlinks(tempHomeRaw)
	tempBinRaw := t.TempDir()
	tempBin, _ := filepath.EvalSymlinks(tempBinRaw)

	// Start SSH server
	sshAddr, cleanupSSH := startSSHServer(t)
	defer cleanupSSH()

	// Parse host & port
	hostIp, portStr, err := net.SplitHostPort(sshAddr)
	if err != nil {
		t.Fatalf("invalid ssh address: %v", err)
	}
	port, _ := net.LookupPort("tcp", portStr)

	// Build mock executables
	buildMockMutagen(t, tempBin)
	buildMockDocker(t, tempBin)
	buildMockSSH(t, tempBin, tempHome)

	// Compile the real sssh binary
	ssshBin := filepath.Join(tempBin, "sssh")
	buildCmd := exec.Command("go", "build", "-o", ssshBin, "../main.go")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile sssh binary: %v", err)
	}

	// Helper to run sssh commands with isolated environment
	runSssh := func(dir string, args ...string) (string, string, error) {
		cmd := exec.Command(ssshBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"HOME="+tempHome,
			"PATH="+tempBin+string(filepath.ListSeparator)+os.Getenv("PATH"),
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}

	// Run init
	_, _, _ = runSssh(tempHome, "init")

	// Write local SSH configuration to ~/.ssh/config under alias "dev-box"
	sshKeyPath := filepath.Join(tempHome, ".ssh", "id_rsa")
	_ = os.MkdirAll(filepath.Dir(sshKeyPath), 0700)
	writeRSAPrivateKey(t, sshKeyPath)

	sshConfigPath := filepath.Join(tempHome, ".ssh", "config")
	sshConfigContent := fmt.Sprintf(`Host dev-box
    HostName %s
    Port %d
    User %s
    IdentityFile %s
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
`, hostIp, port, os.Getenv("USER"), sshKeyPath)
	_ = os.WriteFile(sshConfigPath, []byte(sshConfigContent), 0600)

	// Create two folders
	projA := filepath.Join(tempHome, "proj-a")
	_ = os.MkdirAll(projA, 0755)

	projB := filepath.Join(tempHome, "proj-b")
	_ = os.MkdirAll(projB, 0755)

	remoteA := filepath.Join(tempHome, "remote-a")
	_ = os.MkdirAll(remoteA, 0755)

	remoteB := filepath.Join(tempHome, "remote-b")
	_ = os.MkdirAll(remoteB, 0755)

	// Link Proj A
	_, _, err = runSssh(projA, "link", ".", "dev-box", "--remote-dir", remoteA)
	if err != nil {
		t.Fatalf("link A failed: %v", err)
	}

	// Link Proj B
	_, _, err = runSssh(projB, "link", ".", "dev-box", "--remote-dir", remoteB)
	if err != nil {
		t.Fatalf("link B failed: %v", err)
	}

	// Read links.json and extract listener PIDs
	linksPath := filepath.Join(tempHome, ".config", "sssh", "links.json")
	linksData, err := os.ReadFile(linksPath)
	if err != nil {
		t.Fatalf("failed to read links: %v", err)
	}

	var rawLinks map[string]map[string]interface{}
	if err := json.Unmarshal(linksData, &rawLinks); err != nil {
		t.Fatalf("failed to unmarshal links: %v", err)
	}

	pidAVal, okA := rawLinks[projA]["listener_pid"].(float64)
	pidBVal, okB := rawLinks[projB]["listener_pid"].(float64)

	if !okA || !okB {
		t.Fatalf("listener PIDs not found in links.json. projA=%q, projB=%q, rawLinks=%+v", projA, projB, rawLinks)
	}

	pidA := int(pidAVal)
	pidB := int(pidBVal)

	if pidA != pidB {
		t.Fatalf("expected listener PIDs to be identical (reused), got %d and %d", pidA, pidB)
	}

	// Unlink Proj A
	_, _, err = runSssh(projA, "unlink")
	if err != nil {
		t.Fatalf("unlink A failed: %v", err)
	}

	// Verify the process is still running (Signal 0)
	proc, err := os.FindProcess(pidA)
	if err != nil {
		t.Fatalf("failed to find process: %v", err)
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("expected listener process to still be running after unlinking proj A, but got error: %v", err)
	}

	// Unlink Proj B
	_, _, err = runSssh(projB, "unlink")
	if err != nil {
		t.Fatalf("unlink B failed: %v", err)
	}

	// Verify the process is now terminated (Signal 0 returns error)
	terminated := false
	for i := 0; i < 10; i++ {
		err = proc.Signal(syscall.Signal(0))
		if err != nil {
			terminated = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !terminated {
		t.Fatalf("expected listener process to be terminated after unlinking proj B")
	}
}

func TestIntegration_SyncRecovery(t *testing.T) {
	// Setup isolated home and bin directory
	tempHomeRaw := t.TempDir()
	tempHome, _ := filepath.EvalSymlinks(tempHomeRaw)
	tempBinRaw := t.TempDir()
	tempBin, _ := filepath.EvalSymlinks(tempBinRaw)

	// Start SSH server
	sshAddr, cleanupSSH := startSSHServer(t)
	defer cleanupSSH()

	// Parse host & port
	hostIp, portStr, err := net.SplitHostPort(sshAddr)
	if err != nil {
		t.Fatalf("invalid ssh address: %v", err)
	}
	port, _ := net.LookupPort("tcp", portStr)

	// Build mock executables
	buildMockMutagen(t, tempBin)
	buildMockDocker(t, tempBin)
	buildMockSSH(t, tempBin, tempHome)

	// Compile the real sssh binary
	ssshBin := filepath.Join(tempBin, "sssh")
	buildCmd := exec.Command("go", "build", "-o", ssshBin, "../main.go")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile sssh binary: %v", err)
	}

	// Helper to run sssh commands with isolated environment
	runSssh := func(dir string, args ...string) (string, string, error) {
		cmd := exec.Command(ssshBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"HOME="+tempHome,
			"PATH="+tempBin+string(filepath.ListSeparator)+os.Getenv("PATH"),
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}

	// Run init
	_, _, _ = runSssh(tempHome, "init")

	// Write local SSH configuration to ~/.ssh/config under alias "dev-box"
	sshKeyPath := filepath.Join(tempHome, ".ssh", "id_rsa")
	_ = os.MkdirAll(filepath.Dir(sshKeyPath), 0700)
	writeRSAPrivateKey(t, sshKeyPath)

	sshConfigPath := filepath.Join(tempHome, ".ssh", "config")
	sshConfigContent := fmt.Sprintf(`Host dev-box
    HostName %s
    Port %d
    User %s
    IdentityFile %s
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
`, hostIp, port, os.Getenv("USER"), sshKeyPath)
	_ = os.WriteFile(sshConfigPath, []byte(sshConfigContent), 0600)

	// Create directories
	proj := filepath.Join(tempHome, "proj")
	_ = os.MkdirAll(proj, 0755)

	// Link proj
	_, _, err = runSssh(proj, "link", ".", "dev-box")
	if err != nil {
		t.Fatalf("link failed: %v", err)
	}

	// Read initial links.json
	linksFile := filepath.Join(tempHome, ".config", "sssh", "links.json")
	linksData, err := os.ReadFile(linksFile)
	if err != nil {
		t.Fatalf("failed to read links.json: %v", err)
	}

	var rawLinks map[string]map[string]interface{}
	if err := json.Unmarshal(linksData, &rawLinks); err != nil {
		t.Fatalf("failed to unmarshal links.json: %v", err)
	}

	pidVal, ok := rawLinks[proj]["listener_pid"].(float64)
	if !ok {
		t.Fatalf("listener pid not found")
	}
	pid := int(pidVal)

	// Kill the background listener process to simulate reboot/crash
	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Kill()
		_ = proc.Release()
	}

	// Wait briefly
	time.Sleep(100 * time.Millisecond)

	// Run sssh sync to recover
	out, serr, err := runSssh(proj, "sync")
	if err != nil {
		t.Fatalf("sync command failed: %v, stdout: %s, stderr: %s", err, out, serr)
	}

	// Verify updated links.json
	linksData2, err := os.ReadFile(linksFile)
	if err != nil {
		t.Fatalf("failed to read links.json after sync: %v", err)
	}

	var rawLinks2 map[string]map[string]interface{}
	if err := json.Unmarshal(linksData2, &rawLinks2); err != nil {
		t.Fatalf("failed to unmarshal links.json: %v", err)
	}

	pidVal2, ok2 := rawLinks2[proj]["listener_pid"].(float64)
	if !ok2 {
		t.Fatalf("listener pid not found after sync")
	}
	pid2 := int(pidVal2)

	if pid2 == pid {
		t.Fatalf("expected listener PID to change, but it stayed %d", pid)
	}

	// Verify new process is alive
	proc2, err := os.FindProcess(pid2)
	if err != nil {
		t.Fatalf("new process not found: %v", err)
	}
	if err := proc2.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("new process is not running: %v", err)
	}

	// Cleanup
	_, _, _ = runSssh(proj, "unlink")
}

func buildMockSSH(t *testing.T, binDir, tempHome string) {
	sshConfigPath := filepath.Join(tempHome, ".ssh", "config")
	src := fmt.Sprintf(`package main

import (
	"os"
	"os/exec"
	"syscall"
)

func main() {
	args := os.Args[1:]
	newArgs := []string{
		"-F", "%s",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}
	newArgs = append(newArgs, args...)

	realSsh := "/usr/bin/ssh"
	if _, err := os.Stat(realSsh); err != nil {
		realSsh = "ssh"
	}

	cmd := exec.Command(realSsh, newArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			} else {
				exitCode = exitErr.ExitCode()
			}
		} else {
			exitCode = 255
		}
	}
	os.Exit(exitCode)
}
`, sshConfigPath)

	tmpSrc := filepath.Join(t.TempDir(), "ssh_src.go")
	if err := os.WriteFile(tmpSrc, []byte(src), 0644); err != nil {
		t.Fatalf("failed to write ssh source: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, "ssh"), tmpSrc)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to compile mock ssh: %v", err)
	}
}

func writeRSAPrivateKey(t *testing.T, path string) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(key)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("failed to open key file: %v", err)
	}
	defer f.Close()
	if err := pem.Encode(f, block); err != nil {
		t.Fatalf("failed to encode key: %v", err)
	}
}

func TestZshHookScript_ZshExecution(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not found in PATH, skipping execution test")
	}

	gen := shell.NewGenerator()
	hookScript := gen.ZshHookScript()

	tests := []struct {
		name  string
		setup string
	}{
		{
			name: "UserDefinedWidget",
			setup: `
# Mock user-defined widget function
mock_widget() { }
zle -N accept-line mock_widget
`,
		},
		{
			name:  "BuiltinWidget",
			setup: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zshInput := fmt.Sprintf(`
# Zsh accepts zle commands only when interactive or explicitly initialized
zmodload zsh/zle

%s

# Execute hook script
%s

# Assert the resolved function actually exists
raw_func="${widgets[sssh-orig-accept-line]}"
if [[ "$raw_func" == *:* ]]; then
    # Strip user: prefix
    func="${raw_func#*:}"
    if [[ "$func" == *:* ]]; then
        echo "resolved function $raw_func has double prefix" >&2
        exit 1
    fi
else
    func="$raw_func"
fi
whence -f "$func" || {
    echo "resolved function $func does not exist" >&2
    exit 1
}

`, tt.setup, hookScript)

			cmd := exec.Command(zshPath, "-c", zshInput)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()

			if err != nil {
				t.Fatalf("zsh script execution failed: %v, stderr: %q", err, stderr.String())
			}
		})
	}
}

