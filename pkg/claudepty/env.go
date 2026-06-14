package claudepty

import (
	"os"
	"strings"
)

// providerEnvKeys are the env vars that would force interactive claude
// onto a provider-API auth flow instead of the user's subscription.
// SubscriptionEnv strips them so the spawned claude definitively uses
// the local login.
var providerEnvKeys = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_BASE_URL",
}

// nestingEnvKeys + nestingEnvPrefixes are the markers Claude Code sets in a
// session's environment to flag a nested/child invocation and tie it to the
// parent session. If claude-p is itself launched from inside a Claude Code
// session (e.g. the MCP-bridge "outer claude drives an inner claude" use case),
// the spawned claude would inherit these, run as a child of the parent session,
// and NOT persist its own transcript — which is exactly the transcript claude-p
// reads the answer from. Strip them so the spawned claude is a clean top-level
// session.
var nestingEnvKeys = []string{
	"CLAUDECODE",
}

var nestingEnvPrefixes = []string{
	"CLAUDE_CODE_",
}

// SubscriptionEnv returns os.Environ() with provider-API keys removed
// and a TUI-friendly TERM + NO_COLOR added. Use this when spawning
// claude under your own control; the bare os.Environ() can leak an
// ANTHROPIC_API_KEY that some users have set for other tools, which
// would silently change which account / billing surface gets charged.
func SubscriptionEnv() []string {
	return SubscriptionEnvFrom(os.Environ())
}

// SubscriptionEnvFrom is the testable form of SubscriptionEnv: takes
// the source env explicitly. Empty source is allowed.
func SubscriptionEnvFrom(src []string) []string {
	strip := make(map[string]struct{}, len(providerEnvKeys)+len(nestingEnvKeys))
	for _, k := range providerEnvKeys {
		strip[k] = struct{}{}
	}
	for _, k := range nestingEnvKeys {
		strip[k] = struct{}{}
	}
	out := make([]string, 0, len(src)+2)
	hasTerm := false
	hasNoColor := false
	for _, kv := range src {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if _, drop := strip[k]; drop {
			continue
		}
		if hasAnyPrefix(k, nestingEnvPrefixes) {
			continue
		}
		switch k {
		case "TERM":
			hasTerm = true
		case "NO_COLOR":
			hasNoColor = true
		}
		out = append(out, kv)
	}
	if !hasTerm {
		out = append(out, "TERM=xterm-256color")
	}
	if !hasNoColor {
		out = append(out, "NO_COLOR=1")
	}
	return out
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
