package contract

import (
	"os"
	"strings"
	"testing"
)

func TestSharedWorkProtocolParity(t *testing.T) {
	const heading = "## Shared work protocol (Claude and Codex)"
	agents := readMarkdownSection(t, "../../AGENTS.md", heading)
	claude := readMarkdownSection(t, "../../CLAUDE.md", heading)

	if agents != claude {
		t.Fatal("AGENTS.md and CLAUDE.md must contain the exact same Shared Work Protocol section")
	}
}

func readMarkdownSection(t *testing.T, path, heading string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	start := strings.Index(text, heading+"\n")
	if start == -1 {
		t.Fatalf("%s is missing %q", path, heading)
	}

	section := text[start:]
	if next := strings.Index(section[len(heading)+1:], "\n## "); next != -1 {
		section = section[:len(heading)+1+next+1]
	}

	return strings.TrimSpace(section)
}
