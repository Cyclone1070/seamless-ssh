package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cyclone1070/sssh/internal/config"
	"github.com/Cyclone1070/sssh/internal/domain"
	internalexec "github.com/Cyclone1070/sssh/internal/exec"
	"github.com/Cyclone1070/sssh/internal/fs"
	"github.com/Cyclone1070/sssh/internal/ssh"
	"github.com/Cyclone1070/sssh/internal/sync"
	"github.com/spf13/cobra"
)

var Version = "0.2.0"

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "sssh",
		Version: Version,
		Short:   "Seamless-SSH (sssh) developer workflow offloading tool",
		Long:    "SSSH links local folders to a remote server, syncs edits, intercepts command patterns, runs them remotely, and forwards outputs and container ports.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	rootCmd.InitDefaultVersionFlag()
	if versionFlag := rootCmd.Flags().Lookup("version"); versionFlag != nil {
		versionFlag.Shorthand = "v"
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:                "init",
		Short:              "Initialize sssh configurations and Zsh hook",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runInit()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "link",
		Short:              "Link folder to remote host",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runLink()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "unlink",
		Short:              "Remove link from folder",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runUnlink()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "add",
		Short:              "Add remote command pattern",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runAdd()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "remove",
		Short:              "Remove remote command pattern",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runRemove()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "status",
		Short:              "Show active links and status",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runStatus()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "check-intercept",
		Short:              "Internal command to check if a command pattern should be intercepted",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runCheckIntercept()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "port-listener",
		Short:              "Internal command to listen for container port events and forward them",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runPortListener()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:                "sync",
		Short:              "Restore all links, syncs, and port listeners",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			runSync()
		},
	})

	return rootCmd
}

func PrintUsage() {
	rootCmd := NewRootCmd()
	_ = rootCmd.Help()
}

func Execute() error {
	rootCmd := NewRootCmd()
	return rootCmd.Execute()
}

func ExecuteDirect(args []string) {
	runDirectExec(args)
}

func getPaths() (string, string) {
	home := os.Getenv("HOME")
	configDir := filepath.Join(home, ".config", "sssh")
	configFile := filepath.Join(configDir, "config.yaml")
	linksFile := filepath.Join(configDir, "links.json")
	return configFile, linksFile
}

func findParentLink(linksFile, pwd string) (string, domain.Link, bool) {
	mgr := config.NewManager(fs.NewRealFS())
	links, err := mgr.ReadLinks(linksFile)
	if err != nil {
		return "", domain.Link{}, false
	}

	cleanPwd := filepath.Clean(pwd)
	for localPath, link := range links {
		cleanLocal := filepath.Clean(localPath)
		if cleanPwd == cleanLocal {
			return localPath, link, true
		}
		prefix := cleanLocal + string(filepath.Separator)
		if strings.HasPrefix(cleanPwd, prefix) {
			return localPath, link, true
		}
	}
	return "", domain.Link{}, false
}

func runDirectExec(args []string) {
	_, linksFile := getPaths()
	pwd, _ := os.Getwd()

	linkPath, link, found := findParentLink(linksFile, pwd)
	if !found {
		fmt.Println("No active link found in this directory hierarchy")
		os.Exit(1)
	}

	resolver := ssh.NewResolver()
	remoteDir, err := resolver.Resolve(pwd, link)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	runner := internalexec.NewRealRunner()
	syncMgr := sync.NewManager(runner)
	_ = syncMgr.WaitSync(linkPath, 10*time.Second)

	sshMgr := ssh.NewManager(runner)
	code, err := sshMgr.Exec(link.RemoteHost, remoteDir, args, os.Environ(), os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Printf("Execution error: %v\n", err)
		os.Exit(code)
	}
	os.Exit(code)
}

func controlPath(sshTarget string) string {
	var sb strings.Builder
	for _, r := range sshTarget {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	return filepath.Join("/tmp", fmt.Sprintf("sssh-%s", sb.String()))
}
