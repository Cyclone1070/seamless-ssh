package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/seamless-ssh/sssh/internal/config"
	"github.com/seamless-ssh/sssh/internal/domain"
	internalexec "github.com/seamless-ssh/sssh/internal/exec"
	"github.com/seamless-ssh/sssh/internal/fs"
	"github.com/seamless-ssh/sssh/internal/sync"
)

func runLink() {
	if len(os.Args) < 3 || os.Args[2] != "." {
		fmt.Println("Usage: sssh link . [remote-host] [--remote-dir <dir>]")
		os.Exit(1)
	}

	configFile, linksFile := getPaths()
	mgr := config.NewManager(fs.NewRealFS())

	cfg, err := mgr.ReadConfig(configFile)
	if err != nil {
		fmt.Printf("Error reading config.yaml: %v\n", err)
		os.Exit(1)
	}

	// Determine host
	var host domain.HostConfig
	hostAlias := ""
	if len(os.Args) >= 4 && !strings.HasPrefix(os.Args[3], "-") {
		hostAlias = os.Args[3]
	}

	if hostAlias != "" {
		found := false
		for _, h := range cfg.Hosts {
			if h.Alias == hostAlias {
				host = h
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Host alias %q not found in config.yaml\n", hostAlias)
			os.Exit(1)
		}
	} else {
		if len(cfg.Hosts) == 1 {
			host = cfg.Hosts[0]
		} else {
			fmt.Println("Ambiguous host. Please specify host alias: sssh link . [alias]")
			os.Exit(1)
		}
	}

	// Parse remote dir override
	remoteDir := ""
	pwd, _ := os.Getwd()
	for i, arg := range os.Args {
		if arg == "--remote-dir" && i+1 < len(os.Args) {
			remoteDir = os.Args[i+1]
		}
	}
	if remoteDir == "" {
		remoteDir = "~/.sssh/sync/" + filepath.Base(pwd)
	}

	// Establish SSH ControlMaster tunnel proactively
	portStr := strconv.Itoa(host.Port)
	if host.Port <= 0 {
		portStr = "22"
	}
	controlPath := filepath.Join("/tmp", fmt.Sprintf("sssh-%s-%s-%s", host.User, host.Host, portStr))
	sshArgs := []string{
		"-M", "-N", "-f",
		"-o", "ControlPath=" + controlPath,
		"-o", "ControlPersist=1h",
		"-p", portStr,
	}
	if host.SSHKeyPath != "" {
		sshArgs = append(sshArgs, "-i", host.SSHKeyPath)
	}
	sshArgs = append(sshArgs, host.User+"@"+host.Host)
	_ = exec.Command("ssh", sshArgs...).Run()

	// Start mutagen sync
	runner := internalexec.NewRealRunner()
	syncMgr := sync.NewManager(runner)
	err = syncMgr.Start(pwd, host, remoteDir)
	if err != nil {
		fmt.Printf("Error starting sync: %v\n", err)
		os.Exit(1)
	}

	// Check if there is an existing active listener for this host
	var existingPid int
	linksData, readErr := os.ReadFile(linksFile)
	if readErr == nil {
		var rawLinks map[string]map[string]interface{}
		if json.Unmarshal(linksData, &rawLinks) == nil {
			for _, entry := range rawLinks {
				if entry["remote_host"] == host.Alias {
					if pidVal, ok := entry["listener_pid"].(float64); ok && pidVal > 0 {
						pid := int(pidVal)
						proc, err := os.FindProcess(pid)
						if err == nil {
							if err := proc.Signal(syscall.Signal(0)); err == nil {
								existingPid = pid
								break
							}
						}
					}
				}
			}
		}
	}

	listenerPid := existingPid
	if listenerPid == 0 {
		// Launch background port-listener
		binaryPath, _ := exec.LookPath(os.Args[0])
		if binaryPath == "" {
			binaryPath = os.Args[0] // fallback
		}

		cmd := exec.Command(binaryPath, "port-listener")
		cmd.Dir = pwd
		cmd.Env = append(os.Environ(), "SSSH_LINK_LOCAL="+pwd)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach process
		err = cmd.Start()
		if err == nil {
			listenerPid = cmd.Process.Pid
			go func() { _ = cmd.Wait() }()
		}
	}

	// Write link registry
	link := domain.Link{
		LocalPath:   pwd,
		RemoteHost:  host.Alias,
		RemotePath:  remoteDir,
		Patterns:    []string{}, // opt-in by default
		ListenerPid: listenerPid,
	}

	_ = mgr.WriteLink(linksFile, link)

	fmt.Printf("Successfully linked %s to %s:%s\n", pwd, host.Alias, remoteDir)
}
