package shell

import (
	"path/filepath"
	"strings"
)

var localExclusions = map[string]bool{
	"cd": true, "pushd": true, "popd": true, "dirs": true,
	"exit": true, "logout": true, "exec": true,
	"jobs": true, "fg": true, "bg": true, "wait": true, "disown": true, "kill": true,
	"export": true, "set": true, "unset": true, "typeset": true, "local": true, "declare": true, "integer": true, "float": true, "readonly": true,
	"alias": true, "unalias": true, "source": true, ".": true, "bindkey": true, "compdef": true, "autoload": true, "functions": true, "unfunction": true,
	"fc": true, "history": true, "vared": true, "read": true,
	"ulimit": true, "umask": true,
	"sssh": true, "mutagen": true,
}

type Matcher struct{}

func NewMatcher() *Matcher {
	return &Matcher{}
}

func stripEnv(cmdLine string) string {
	words := strings.Fields(cmdLine)
	var actualCmd []string
	for _, w := range words {
		if strings.Contains(w, "=") {
			continue
		}
		actualCmd = append(actualCmd, w)
	}
	if len(actualCmd) == 0 {
		return ""
	}
	return strings.Join(actualCmd, " ")
}

func (m *Matcher) ShouldRunRemote(cmdLine string, patterns []string) bool {
	stripped := stripEnv(cmdLine)
	if stripped == "" {
		return false
	}

	fields := strings.Fields(stripped)
	if len(fields) == 0 {
		return false
	}
	firstWord := fields[0]

	if localExclusions[firstWord] {
		return false
	}

	for _, pattern := range patterns {
		p := strings.TrimSpace(pattern)
		c := strings.TrimSpace(stripped)

		if strings.Contains(p, "*") {
			matched, err := filepath.Match(p, c)
			if err == nil && matched {
				return true
			}
		} else {
			if p == c {
				return true
			}
			if strings.HasPrefix(c, p+" ") || strings.HasPrefix(c, p+"\t") {
				return true
			}
		}
	}

	return false
}

const zshHookTemplate = `# sssh Zsh Integration Hook

if [[ "$widgets[accept-line]" != "user:sssh-accept-line" ]]; then
    zle -N sssh-orig-accept-line "${widgets[accept-line]:-.accept-line}"
    zle -N accept-line sssh-accept-line
fi

sssh-accept-line() {
    if sssh check-intercept "$BUFFER" "$PWD"; then
        BUFFER="sssh $BUFFER"
    fi
    zle sssh-orig-accept-line
}

# Run sync in background on terminal startup to restore links, syncs, and listeners
(sssh sync >/dev/null 2>&1 &)
`

type Generator struct{}

func NewGenerator() *Generator {
	return &Generator{}
}

func (g *Generator) ZshHookScript() string {
	return zshHookTemplate
}
