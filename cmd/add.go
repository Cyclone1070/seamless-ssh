package cmd

import (
	"fmt"
	"os"

	"github.com/Cyclone1070/seamless-ssh/internal/config"
	"github.com/Cyclone1070/seamless-ssh/internal/fs"
)

func runAdd() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: sssh add \"<pattern>\"")
		os.Exit(1)
	}
	pattern := os.Args[2]

	_, linksFile := getPaths()
	pwd, _ := os.Getwd()

	linkPath, link, found := findParentLink(linksFile, pwd)
	if !found {
		fmt.Println("No active link found in this directory hierarchy")
		os.Exit(1)
	}

	exists := false
	for _, p := range link.Patterns {
		if p == pattern {
			exists = true
			break
		}
	}

	if !exists {
		link.Patterns = append(link.Patterns, pattern)
		mgr := config.NewManager(fs.NewRealFS())
		_ = mgr.WriteLink(linksFile, link)
		fmt.Printf("Added pattern %q to remote execution rules for %s\n", pattern, linkPath)
	} else {
		fmt.Printf("Pattern %q already exists in remote execution rules\n", pattern)
	}
}
