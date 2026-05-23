package claudepty

import "testing"

func TestClassifyInteractiveFailure(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   string
	}{
		{"empty", "", ""},
		{"benign welcome", "Welcome back\n❯ ", ""},
		{"failed to auth", "❯ \nfailed to authenticate", FailureAuthBlocked},
		{"api 403", "Some text\nAPI Error: 403\nmore", FailureAuthBlocked},
		{"please run /login", "your session expired. please run /login", FailureAuthBlocked},
		{"hit limit", "You've hit your limit for the next 5 hours", FailureRateLimit},
		{"approaching limit", "Approaching usage limit", FailureRateLimit},
		{"5-hour limit", "5-hour limit reached", FailureRateLimit},
		{"trust folder", "Do you trust the files in this folder?", FailureWorkspaceTrustBlocked},
		{"permission allow", "Permission required. Allow this tool? (y/N)", FailureToolApprovalBlocked},
		{"permission deny", "Permission required. Deny once?", FailureToolApprovalBlocked},
		// Negative: "permission" alone shouldn't match.
		{"permission word alone", "permissions are read-only", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyInteractiveFailure(c.screen)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
