package claudep

import "testing"

func TestCompactScreen(t *testing.T) {
	in := "  \n" + "hello   \n" + "   \n" + "world \n" + "   \n"
	if got := CompactScreen(in); got != "hello\nworld" {
		t.Errorf("CompactScreen = %q, want %q", got, "hello\nworld")
	}
	if got := CompactScreen("   \n  \n"); got != "" {
		t.Errorf("all-blank input should compact to empty, got %q", got)
	}
}
