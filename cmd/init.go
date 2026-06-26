package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"fmt"

	"github.com/seamless-ssh/sssh/internal/shell"
)

func runInit() {
	configFile, _ := getPaths()
	configDir := filepath.Dir(configFile)

	// Ensure config directory exists
	_ = os.MkdirAll(configDir, 0755)

	// Write default config if not exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		defaultConfig := []byte(`hosts:
  - alias: dev-box
    host: 127.0.0.1
    port: 22
    user: ubuntu
    ssh_key_path: ~/.ssh/id_rsa
`)
		_ = os.WriteFile(configFile, defaultConfig, 0644)
		fmt.Printf("Created default config template at %s\n", configFile)
	}

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
