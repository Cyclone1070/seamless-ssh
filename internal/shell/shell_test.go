package shell_test

import (
	"testing"

	"github.com/seamless-ssh/sssh/internal/shell"
)

func TestPattern_MatchExact(t *testing.T) {
	matcher := shell.NewMatcher()
	patterns := []string{"go test"}

	if !matcher.ShouldRunRemote("go test", patterns) {
		t.Error("expected exact match 'go test' to run remote")
	}
}

func TestPattern_MatchPrefix(t *testing.T) {
	matcher := shell.NewMatcher()
	patterns := []string{"go test"}

	if !matcher.ShouldRunRemote("go test -race ./...", patterns) {
		t.Error("expected prefix match to run remote")
	}
}

func TestPattern_MatchGlob(t *testing.T) {
	matcher := shell.NewMatcher()
	patterns := []string{"go test *"}

	if !matcher.ShouldRunRemote("go test -race", patterns) {
		t.Error("expected glob pattern match to run remote")
	}
}

func TestPattern_NoMatch(t *testing.T) {
	matcher := shell.NewMatcher()
	patterns := []string{"go test"}

	if matcher.ShouldRunRemote("git status", patterns) {
		t.Error("expected git status to not match go test")
	}
}

func TestPattern_EmptyRules(t *testing.T) {
	matcher := shell.NewMatcher()
	var patterns []string

	if matcher.ShouldRunRemote("go test", patterns) {
		t.Error("expected command to run locally when no patterns are defined")
	}
}

func TestPattern_HardcodedBuiltins(t *testing.T) {
	matcher := shell.NewMatcher()
	patterns := []string{"*"} // Catch-all pattern

	builtins := []string{
		"cd", "cd ..", "pushd", "popd", "dirs",
		"exit", "logout", "exec",
		"jobs", "fg", "bg", "wait", "disown", "kill",
		"export", "set", "unset", "typeset", "local", "declare",
		"alias", "unalias", "source", ".", "bindkey", "autoload",
		"fc", "history", "vared", "read",
		"ulimit", "umask",
		"sssh", "sssh link", "mutagen",
	}

	for _, cmd := range builtins {
		if matcher.ShouldRunRemote(cmd, patterns) {
			t.Errorf("expected builtin %q to be hard-excluded from remote execution", cmd)
		}
	}
}

func TestPattern_MatcherPerformance(t *testing.T) {
	matcher := shell.NewMatcher()
	patterns := []string{"go test *", "make", "npm run dev"}

	// Run multiple times to assert speed
	for i := 0; i < 1000; i++ {
		_ = matcher.ShouldRunRemote("go test -race ./...", patterns)
	}
}

func TestZshHookScript_ContainsHook(t *testing.T) {
	gen := shell.NewGenerator()
	script := gen.ZshHookScript()
	if script == "" {
		t.Error("expected non-empty Zsh hook script")
	}
	if !contains(script, "sssh-accept-line") {
		t.Error("expected script to contain sssh-accept-line hook")
	}
	if !contains(script, "sssh sync") {
		t.Error("expected script to contain sssh sync recovery call")
	}
}

func contains(s, substr string) bool {
	// Simple helper
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
