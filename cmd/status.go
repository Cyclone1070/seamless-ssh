package cmd

import (
	"fmt"

	"github.com/seamless-ssh/sssh/internal/config"
	"github.com/seamless-ssh/sssh/internal/fs"
)

func runStatus() {
	_, linksFile := getPaths()
	mgr := config.NewManager(fs.NewRealFS())
	links, err := mgr.ReadLinks(linksFile)
	if err != nil || len(links) == 0 {
		fmt.Println("No active links registered.")
		return
	}

	fmt.Println("Active Folder Links:")
	for localPath, link := range links {
		fmt.Printf("  Local:  %s\n", localPath)
		fmt.Printf("  Remote: %s:%s\n", link.RemoteHost, link.RemotePath)
		fmt.Printf("  Rules:  %v\n\n", link.Patterns)
	}
}
