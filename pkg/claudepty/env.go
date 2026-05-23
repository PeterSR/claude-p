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
	strip := make(map[string]struct{}, len(providerEnvKeys))
	for _, k := range providerEnvKeys {
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
