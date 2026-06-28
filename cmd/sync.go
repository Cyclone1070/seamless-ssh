package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"syscall"

	"github.com/Cyclone1070/seamless-ssh/internal/config"
	"github.com/Cyclone1070/seamless-ssh/internal/domain"
	internalexec "github.com/Cyclone1070/seamless-ssh/internal/exec"
	"github.com/Cyclone1070/seamless-ssh/internal/fs"
	"github.com/Cyclone1070/seamless-ssh/internal/sync"
)

func runSync() {
	_, linksFile := getPaths()
	mgr := config.NewManager(fs.NewRealFS())

	links, err := mgr.ReadLinks(linksFile)
	if err != nil {
		// No links to sync
		return
	}

	if len(links) == 0 {
		return
	}

	runner := internalexec.NewRealRunner()
	syncMgr := sync.NewManager(runner)

	// Keep track of active listener PIDs by host target
	activePidsByHost := make(map[string]int)

	// Phase 1: Determine any currently running listener PIDs from links.json
	for _, link := range links {
		if link.ListenerPid > 0 {
			proc, err := os.FindProcess(link.ListenerPid)
			if err == nil {
				if err := proc.Signal(syscall.Signal(0)); err == nil {
					activePidsByHost[link.RemoteHost] = link.ListenerPid
				}
			}
		}
	}

	// Phase 2: Restore each link
	updatedLinks := make(map[string]domain.Link)
	for localPath, link := range links {
		// 1. Proactively verify and establish SSH ControlMaster tunnel
		controlPath := controlPath(link.RemoteHost)
		sshArgs := []string{
			"-M", "-N", "-f",
			"-o", "ControlPath=" + controlPath,
			"-o", "ControlPersist=1h",
			link.RemoteHost,
		}
		_ = exec.Command("ssh", sshArgs...).Run()

		// 2. Restart Mutagen session
		_ = syncMgr.Start(localPath, link.RemoteHost, link.RemotePath)

		// 3. Verify/Restart port-listener daemon
		pid := activePidsByHost[link.RemoteHost]
		if pid == 0 {
			// Spawn a new listener
			binaryPath, _ := exec.LookPath(os.Args[0])
			if binaryPath == "" {
				binaryPath = os.Args[0]
			}

			cmd := exec.Command(binaryPath, "port-listener")
			cmd.Dir = localPath
			cmd.Env = append(os.Environ(), "SSSH_LINK_LOCAL="+localPath)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			err = cmd.Start()
			if err == nil {
				pid = cmd.Process.Pid
				activePidsByHost[link.RemoteHost] = pid
				go func() { _ = cmd.Wait() }()
			}
		}

		link.ListenerPid = pid
		updatedLinks[localPath] = link
	}

	// Rewrite links.json with updated listener PIDs
	linksData, err := json.MarshalIndent(updatedLinks, "", "  ")
	if err == nil {
		_ = os.WriteFile(linksFile, linksData, 0644)
	}
}
