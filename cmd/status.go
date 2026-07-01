package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Cyclone1070/sssh/internal/config"
	"github.com/Cyclone1070/sssh/internal/fs"
)

func runStatus() {
	_, linksFile := getPaths()
	runStatusWithWriter(os.Stdout, linksFile)
}

func runStatusWithWriter(w io.Writer, linksFile string) {
	mgr := config.NewManager(fs.NewRealFS())
	links, err := mgr.ReadLinks(linksFile)
	if err != nil || len(links) == 0 {
		fmt.Fprintln(w, "No active links registered.")
		return
	}

	fmt.Fprintln(w, "Active Folder Links:")
	for localPath, link := range links {
		fmt.Fprintf(w, "  Local:  %s\n", localPath)
		fmt.Fprintf(w, "  Remote: %s:%s\n", link.RemoteHost, link.RemotePath)

		var quoted []string
		for _, p := range link.Patterns {
			quoted = append(quoted, fmt.Sprintf("%q", p))
		}
		fmt.Fprintf(w, "  Rules:  [%s]\n\n", strings.Join(quoted, ", "))
	}
}
