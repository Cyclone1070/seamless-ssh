package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"fmt"

	"github.com/Cyclone1070/sssh/internal/shell"
)

func runInit() {
	_, linksFile := getPaths()
	configDir := filepath.Dir(linksFile)

	// Ensure config directory exists
	_ = os.MkdirAll(configDir, 0755)

	// Append or update Zsh integration
	zshrc := filepath.Join(os.Getenv("HOME"), ".zshrc")
	gen := shell.NewGenerator()
	hookScript := gen.ZshHookScript()

	if err := runInitWithZshrc(zshrc, hookScript); err != nil {
		fmt.Printf("Error configuring Zsh integration: %v\n", err)
		os.Exit(1)
	}
}

func runInitWithZshrc(zshrcPath string, hookScript string) error {
	contentBytes, _ := os.ReadFile(zshrcPath)
	content := string(contentBytes)
	marker := "# sssh Zsh Integration Hook"
	var newContent string

	if strings.Contains(content, marker) {
		idx := strings.Index(content, marker)
		before := strings.TrimRight(content[:idx], "\r\n ")
		newContent = before + "\n\n" + hookScript + "\n"
	} else {
		if content != "" && !strings.HasSuffix(content, "\n") {
			newContent = content + "\n\n" + hookScript + "\n"
		} else if content != "" {
			newContent = content + "\n" + hookScript + "\n"
		} else {
			newContent = hookScript + "\n"
		}
	}

	if content == newContent {
		fmt.Println("Zsh integration hook is already configured in ~/.zshrc")
		return nil
	}

	err := os.WriteFile(zshrcPath, []byte(newContent), 0644)
	if err != nil {
		return err
	}

	if strings.Contains(content, marker) {
		fmt.Println("Updated Zsh integration hook in ~/.zshrc. Restart your shell to apply changes.")
	} else {
		fmt.Println("Added Zsh integration hook to ~/.zshrc. Restart your shell to apply changes.")
	}
	return nil
}

