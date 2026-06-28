package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"fmt"

	"github.com/Cyclone1070/seamless-ssh/internal/shell"
)

func runInit() {
	_, linksFile := getPaths()
	configDir := filepath.Dir(linksFile)

	// Ensure config directory exists
	_ = os.MkdirAll(configDir, 0755)

	// Append Zsh integration
	zshrc := filepath.Join(os.Getenv("HOME"), ".zshrc")
	content, err := os.ReadFile(zshrc)
	if err == nil && !strings.Contains(string(content), "sssh Zsh Integration Hook") {
		gen := shell.NewGenerator()
		hook := "\n" + gen.ZshHookScript()
		f, err := os.OpenFile(zshrc, os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = f.WriteString(hook)
			_ = f.Close()
			fmt.Println("Added Zsh integration hook to ~/.zshrc. Restart your shell to apply changes.")
		}
	} else {
		fmt.Println("Zsh integration hook is already configured in ~/.zshrc")
	}
}
