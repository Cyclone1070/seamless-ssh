package cmd

import (
	"fmt"
	"os"

	"github.com/seamless-ssh/sssh/internal/config"
	"github.com/seamless-ssh/sssh/internal/fs"
)

func runRemove() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: sssh remove \"<pattern>\"")
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

	newPatterns := []string{}
	removed := false
	for _, p := range link.Patterns {
		if p == pattern {
			removed = true
			continue
		}
		newPatterns = append(newPatterns, p)
	}

	if removed {
		link.Patterns = newPatterns
		mgr := config.NewManager(fs.NewRealFS())
		_ = mgr.WriteLink(linksFile, link)
		fmt.Printf("Removed pattern %q from remote execution rules for %s\n", pattern, linkPath)
	} else {
		fmt.Printf("Pattern %q not found in remote execution rules\n", pattern)
	}
}
