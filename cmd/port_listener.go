package cmd

import (
	"os"

	"github.com/seamless-ssh/sssh/internal/config"
	"github.com/seamless-ssh/sssh/internal/domain"
	internalexec "github.com/seamless-ssh/sssh/internal/exec"
	"github.com/seamless-ssh/sssh/internal/fs"
	"github.com/seamless-ssh/sssh/internal/portforward"
)

func runPortListener() {
	pwd := os.Getenv("SSSH_LINK_LOCAL")
	if pwd == "" {
		pwd, _ = os.Getwd()
	}

	configFile, linksFile := getPaths()
	mgr := config.NewManager(fs.NewRealFS())

	_, link, found := findParentLink(linksFile, pwd)
	if !found {
		os.Exit(1)
	}

	cfg, err := mgr.ReadConfig(configFile)
	if err != nil {
		os.Exit(1)
	}

	var host domain.HostConfig
	for _, h := range cfg.Hosts {
		if h.Alias == link.RemoteHost {
			host = h
			break
		}
	}
	if host.Host == "" {
		os.Exit(1)
	}

	runner := internalexec.NewRealRunner()
	pfMgr := portforward.NewManager(runner, portforward.RealNetProvider{})

	go func() { _ = pfMgr.ListenEvents(host, false) }()
	go func() { _ = pfMgr.ListenEvents(host, true) }()

	select {}
}
