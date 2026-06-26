package main

import (
	"fmt"
	"os"

	"github.com/seamless-ssh/sssh/cmd"
)

func main() {
	if len(os.Args) < 2 {
		cmd.PrintUsage()
		os.Exit(1)
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "init", "link", "unlink", "add", "remove", "status", "check-intercept", "port-listener", "help", "-h", "--help":
		if err := cmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		cmd.ExecuteDirect(os.Args[1:])
	}
}
