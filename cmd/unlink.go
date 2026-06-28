package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Cyclone1070/seamless-ssh/internal/config"
	internalexec "github.com/Cyclone1070/seamless-ssh/internal/exec"
	"github.com/Cyclone1070/seamless-ssh/internal/fs"
	"github.com/Cyclone1070/seamless-ssh/internal/portforward"
	"github.com/Cyclone1070/seamless-ssh/internal/sync"
)

func runUnlink() {
	_, linksFile := getPaths()
	pwd, _ := os.Getwd()

	linkPath, _, found := findParentLink(linksFile, pwd)
	if !found {
		fmt.Println("No active link found in this directory hierarchy")
		os.Exit(1)
	}

	// Stop sync
	runner := internalexec.NewRealRunner()
	syncMgr := sync.NewManager(runner)
	_ = syncMgr.Stop(linkPath)

	// Terminate port listener background process only if no other links use it
	mgr := config.NewManager(fs.NewRealFS())
	links, readErr := mgr.ReadLinks(linksFile)
	if readErr == nil {
		if entry, ok := links[linkPath]; ok && entry.ListenerPid > 0 {
			pidToKill := entry.ListenerPid
			
			// Count references in links
			refCount := 0
			for _, e := range links {
				if e.ListenerPid == pidToKill {
					refCount++
				}
			}
			
			if refCount <= 1 {
				proc, err := os.FindProcess(pidToKill)
				if err == nil {
					_ = proc.Kill()
				}
			}
		}
	}
	delete(links, linkPath)

	data, _ := json.MarshalIndent(links, "", "  ")
	_ = os.WriteFile(linksFile, data, 0644)

	// Stop any active listeners in the unlink process if run directly
	portforward.NewManager(runner, portforward.RealNetProvider{}).StopAll()

	fmt.Printf("Unlinked folder %s\n", linkPath)
}
