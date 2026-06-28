package cmd

import (
	"os"

	"github.com/Cyclone1070/sssh/internal/shell"
)

func runCheckIntercept() {
	if len(os.Args) < 4 {
		os.Exit(1)
	}
	cmdLine := os.Args[2]
	pwd := os.Args[3]

	_, linksFile := getPaths()
	_, link, found := findParentLink(linksFile, pwd)
	if !found {
		os.Exit(1)
	}

	matcher := shell.NewMatcher()
	if matcher.ShouldRunRemote(cmdLine, link.Patterns) {
		os.Exit(0)
	}
	os.Exit(1)
}
