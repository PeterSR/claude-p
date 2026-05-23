package claudepty

import "strings"

// Failure categories ClassifyInteractiveFailure can return. Empty
// string means "nothing recognisable on screen."
const (
	FailureAuthBlocked              = "auth_blocked"
	FailureRateLimit                = "rate_limit"
	FailureWorkspaceTrustBlocked    = "workspace_trust_blocked"
	FailureToolApprovalBlocked      = "tool_approval_blocked"
	FailureCustomAPIKeyDetected     = "custom_api_key_detected"
)

// ClassifyInteractiveFailure returns a short reason string for common
// failure surfaces visible in the rendered TUI, or "" if nothing
// recognisable is present. Cheap substring checks against a lower-cased
// snapshot of the screen.
func ClassifyInteractiveFailure(screen string) string {
	low := strings.ToLower(screen)
	switch {
	case strings.Contains(low, "failed to authenticate"),
		strings.Contains(low, "api error: 403"),
		strings.Contains(low, "please run /login"):
		return FailureAuthBlocked
	case strings.Contains(low, "hit your limit"),
		strings.Contains(low, "approaching usage limit"),
		strings.Contains(low, "5-hour limit"):
		return FailureRateLimit
	case strings.Contains(low, "do you trust") && strings.Contains(low, "folder"):
		return FailureWorkspaceTrustBlocked
	case strings.Contains(low, "detected a custom api key"):
		// Claude pauses with a "Detected a custom API key in your
		// environment / Do you want to use this API key?" modal
		// when it sees ANTHROPIC_API_KEY (or AUTH_TOKEN) in the env
		// and the user previously chose to be asked. Block the
		// orchestrator path so the caller knows to either strip the
		// env (SubscriptionEnv) or accept the modal.
		return FailureCustomAPIKeyDetected
	case strings.Contains(low, "permission") && (strings.Contains(low, "allow") || strings.Contains(low, "deny")):
		return FailureToolApprovalBlocked
	}
	return ""
}
