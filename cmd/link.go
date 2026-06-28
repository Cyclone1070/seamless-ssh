package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Cyclone1070/sssh/internal/config"
	"github.com/Cyclone1070/sssh/internal/domain"
	internalexec "github.com/Cyclone1070/sssh/internal/exec"
	"github.com/Cyclone1070/sssh/internal/fs"
	"github.com/Cyclone1070/sssh/internal/sync"
)

func runLink() {
	if len(os.Args) < 4 || os.Args[2] != "." {
		fmt.Println("Usage: sssh link . [ssh-target] [--remote-dir <dir>]")
		os.Exit(1)
	}

	_, linksFile := getPaths()
	mgr := config.NewManager(fs.NewRealFS())

	sshTarget := ""
	if len(os.Args) >= 4 && !strings.HasPrefix(os.Args[3], "-") {
		sshTarget = os.Args[3]
	}
	if sshTarget == "" {
		fmt.Println("Usage: sssh link . [ssh-target] [--remote-dir <dir>]")
		os.Exit(1)
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
	controlPath := controlPath(sshTarget)
	sshArgs := []string{
		"-M", "-N", "-f",
		"-o", "ControlPath=" + controlPath,
		"-o", "ControlPersist=1h",
		sshTarget,
	}
	_ = exec.Command("ssh", sshArgs...).Run()

	// Start mutagen sync
	runner := internalexec.NewRealRunner()
	syncMgr := sync.NewManager(runner)
	err := syncMgr.Start(pwd, sshTarget, remoteDir)
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
				if entry["remote_host"] == sshTarget {
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
		RemoteHost:  sshTarget,
		RemotePath:  remoteDir,
		Patterns:    []string{}, // opt-in by default
		ListenerPid: listenerPid,
	}

	_ = mgr.WriteLink(linksFile, link)

	fmt.Printf("Successfully linked %s to %s:%s\n", pwd, sshTarget, remoteDir)
}
