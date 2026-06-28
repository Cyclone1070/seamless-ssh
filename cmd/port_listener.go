package cmd

import (
	"os"

	internalexec "github.com/Cyclone1070/sssh/internal/exec"
	"github.com/Cyclone1070/sssh/internal/portforward"
)

func runPortListener() {
	pwd := os.Getenv("SSSH_LINK_LOCAL")
	if pwd == "" {
		pwd, _ = os.Getwd()
	}

	_, linksFile := getPaths()

	_, link, found := findParentLink(linksFile, pwd)
	if !found {
		os.Exit(1)
	}

	runner := internalexec.NewRealRunner()
	pfMgr := portforward.NewManager(runner, portforward.RealNetProvider{})

	go func() { _ = pfMgr.ListenEvents(link.RemoteHost, false) }()
	go func() { _ = pfMgr.ListenEvents(link.RemoteHost, true) }()

	select {}
}
