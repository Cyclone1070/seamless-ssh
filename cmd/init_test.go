package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateZshrc_AppendsIfMissing(t *testing.T) {
	tmpDir := t.TempDir()
	zshrcPath := filepath.Join(tmpDir, ".zshrc")
	initialContent := "export PATH=$PATH:/usr/local/bin\n"
	err := os.WriteFile(zshrcPath, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("failed to write initial zshrc: %v", err)
	}

	hookScript := "# sssh Zsh Integration Hook\necho 'hello sssh'"

	err = runInitWithZshrc(zshrcPath, hookScript)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	contentBytes, err := os.ReadFile(zshrcPath)
	if err != nil {
		t.Fatalf("failed to read zshrc: %v", err)
	}
	content := string(contentBytes)

	if !strings.HasPrefix(content, initialContent) {
		t.Errorf("initial content was modified: %q", content)
	}
	if !strings.Contains(content, hookScript) {
		t.Errorf("expected hook script to be appended, got: %q", content)
	}
}

func TestUpdateZshrc_ReplacesIfExist(t *testing.T) {
	tmpDir := t.TempDir()
	zshrcPath := filepath.Join(tmpDir, ".zshrc")
	initialContent := "export PATH=$PATH:/usr/local/bin\n# sssh Zsh Integration Hook\nOLD HOOK CONTENT HERE"
	err := os.WriteFile(zshrcPath, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("failed to write initial zshrc: %v", err)
	}

	hookScript := "# sssh Zsh Integration Hook\nNEW HOOK CONTENT HERE"

	err = runInitWithZshrc(zshrcPath, hookScript)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	contentBytes, err := os.ReadFile(zshrcPath)
	if err != nil {
		t.Fatalf("failed to read zshrc: %v", err)
	}
	content := string(contentBytes)

	if strings.Contains(content, "OLD HOOK CONTENT HERE") {
		t.Errorf("expected old hook to be removed, got: %q", content)
	}
	if !strings.Contains(content, "NEW HOOK CONTENT HERE") {
		t.Errorf("expected new hook to be present, got: %q", content)
	}
	if !strings.HasPrefix(content, "export PATH=$PATH:/usr/local/bin\n") {
		t.Errorf("expected prefix to be preserved, got: %q", content)
	}
}
