package claudepty

import (
	"strings"
	"testing"
)

func TestSubscriptionEnvFrom_stripsProviderKeys(t *testing.T) {
	src := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=sk-redacted",
		"ANTHROPIC_AUTH_TOKEN=tok-redacted",
		"ANTHROPIC_BASE_URL=https://example.test",
		"USER=alice",
	}
	out := SubscriptionEnvFrom(src)
	joined := strings.Join(out, "\n")
	for _, banned := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"} {
		if strings.Contains(joined, banned+"=") {
			t.Errorf("expected %s to be stripped, got: %s", banned, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin") {
		t.Error("PATH was dropped; should be preserved")
	}
	if !strings.Contains(joined, "USER=alice") {
		t.Error("USER was dropped; should be preserved")
	}
}

func TestSubscriptionEnvFrom_stripsNestingMarkers(t *testing.T) {
	// When claude-p runs from inside a Claude Code session, the spawned claude
	// must not inherit the parent's nesting markers, or it runs as a child and
	// never persists its own transcript.
	src := []string{
		"PATH=/usr/bin",
		"CLAUDECODE=1",
		"CLAUDE_CODE_SESSION_ID=980d98b8-de22-42b6-b17f-5d50cfe5e64d",
		"CLAUDE_CODE_CHILD_SESSION=1",
		"CLAUDE_CODE_ENTRYPOINT=cli",
		"USER=alice",
	}
	joined := strings.Join(SubscriptionEnvFrom(src), "\n")
	for _, banned := range []string{"CLAUDECODE=", "CLAUDE_CODE_SESSION_ID=", "CLAUDE_CODE_CHILD_SESSION=", "CLAUDE_CODE_ENTRYPOINT="} {
		if strings.Contains(joined, banned) {
			t.Errorf("expected %s to be stripped, got: %s", banned, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "USER=alice") {
		t.Error("non-nesting vars should be preserved")
	}
}

func TestSubscriptionEnvFrom_addsTermAndNoColor(t *testing.T) {
	out := SubscriptionEnvFrom([]string{"PATH=/usr/bin"})
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "TERM=xterm-256color") {
		t.Error("TERM was not added")
	}
	if !strings.Contains(joined, "NO_COLOR=1") {
		t.Error("NO_COLOR was not added")
	}
}

func TestSubscriptionEnvFrom_preservesExistingTerm(t *testing.T) {
	out := SubscriptionEnvFrom([]string{"TERM=dumb"})
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "TERM=dumb") {
		t.Error("existing TERM should be preserved")
	}
	if strings.Count(joined, "TERM=") != 1 {
		t.Errorf("expected exactly one TERM= entry, got: %s", joined)
	}
}

func TestSubscriptionEnvFrom_preservesExistingNoColor(t *testing.T) {
	out := SubscriptionEnvFrom([]string{"NO_COLOR=0"})
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "NO_COLOR=0") {
		t.Error("existing NO_COLOR should be preserved")
	}
	if strings.Count(joined, "NO_COLOR=") != 1 {
		t.Errorf("expected exactly one NO_COLOR= entry, got: %s", joined)
	}
}

func TestSubscriptionEnvFrom_handlesMalformedEntries(t *testing.T) {
	// An entry without '=' should be passed through unchanged rather
	// than crashing.
	out := SubscriptionEnvFrom([]string{"PATH=/usr/bin", "WEIRD_NO_EQUALS"})
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "WEIRD_NO_EQUALS") {
		t.Error("malformed entry was dropped")
	}
}
