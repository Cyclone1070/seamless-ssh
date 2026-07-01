package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Cyclone1070/sssh/internal/config"
	"github.com/Cyclone1070/sssh/internal/domain"
	"github.com/Cyclone1070/sssh/internal/fs"
)

func TestRunStatus_QuotedRules(t *testing.T) {
	tmpDir := t.TempDir()
	linksFile := filepath.Join(tmpDir, "links.json")

	// Write mock link configuration with space-containing patterns
	mgr := config.NewManager(fs.NewRealFS())
	link := domain.Link{
		LocalPath:  "/local/path",
		RemoteHost: "dev-box",
		RemotePath: "/remote/path",
		Patterns:   []string{"go test*", "podman *"},
	}
	err := mgr.WriteLink(linksFile, link)
	if err != nil {
		t.Fatalf("failed to write test link: %v", err)
	}

	buf := new(bytes.Buffer)
	runStatusWithWriter(buf, linksFile)

	out := buf.String()
	expectedRulesLine := `  Rules:  ["go test*", "podman *"]`
	if !strings.Contains(out, expectedRulesLine) {
		t.Errorf("expected status output to contain quoted rules: %q, but got: %q", expectedRulesLine, out)
	}
}
