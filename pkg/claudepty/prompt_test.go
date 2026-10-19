package claudepty

import "testing"

func TestHasInputPrompt(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   bool
	}{
		{"bare prompt glyph", "Welcome\n❯ \nstatus", true},
		{"prompt with Try placeholder", "Welcome\n❯ Try \"build me a CLI\"\n", true},
		{"menu option — should not match", "❯ 1. Yes, I trust this folder\n   2. No, exit", false},
		{"option-2 cursor", "  1. Yes\n❯ 2. No, exit", false},
		{"completely empty", "", false},
		{"just text, no prompt", "Hello there", false},
		{"multiple lines, prompt buried", "Header\nbody text\n\n❯ \nfooter", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := HasInputPrompt(c.screen)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestReadyForInput(t *testing.T) {
	screen := func(cursor Cursor, lines ...string) *Screen {
		return &Screen{Lines: lines, Cursor: cursor}
	}
	cases := []struct {
		name string
		scr  *Screen
		want bool
	}{
		{
			"cursor on chevron row with ghost placeholder",
			screen(Cursor{Row: 1, Col: 2, Visible: true}, "header", "❯ Try \"fix typecheck errors\""),
			true,
		},
		{
			"cursor on empty input row",
			screen(Cursor{Row: 0, Col: 2, Visible: true}, "❯ "),
			true,
		},
		{
			"cursor on chevron row, non-Try placeholder still ready",
			screen(Cursor{Row: 0, Col: 2, Visible: true}, "❯ ask me anything"),
			true,
		},
		{
			"cursor hidden — not ready",
			screen(Cursor{Row: 0, Col: 2, Visible: false}, "❯ Try \"x\""),
			false,
		},
		{
			"cursor on trust-modal option row — not ready",
			screen(Cursor{Row: 0, Col: 2, Visible: true}, "❯ 1. Yes, I trust this folder", "  2. No, exit"),
			false,
		},
		{
			"cursor on a non-prompt row — not ready",
			screen(Cursor{Row: 0, Col: 0, Visible: true}, "loading…", "❯ "),
			false,
		},
		{
			"cursor row out of range",
			screen(Cursor{Row: 9, Col: 0, Visible: true}, "❯ "),
			false,
		},
		{"nil screen", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ReadyForInput(c.scr); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestHasStylePicker(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   bool
	}{
		{"first-run picker", "Choose the text style\n❯ Dark mode\n  Light mode\n  Dark mode (colorblind)", true},
		{"only dark mentioned", "Switched to Dark mode", false},
		{"input prompt", "❯ Try \"fix the bug\"", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasStylePicker(c.screen); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestTrustSelection(t *testing.T) {
	cases := []struct {
		name       string
		screen     string
		yes, found bool
	}{
		{"yes preselected", "Do you trust the files in this folder?\n ❯ 1. Yes, I trust this folder\n   2. No, exit", true, true},
		{"no preselected", "Do you trust the files in this folder?\n   1. Yes, I trust this folder\n ❯ 2. No, exit", false, true},
		{"unnumbered, no first (2.1.303)", "Quick safety check: Is this a project you created or one you trust?\n ❯ No, exit\n   Yes, I trust this folder\n Enter to confirm", false, true},
		{"unnumbered, moved to yes", "Quick safety check\n   No, exit\n ❯ Yes, I trust this folder", true, true},
		{"no menu yet", "Do you trust the files in this folder?", false, false},
		{"transcript user message starting with no", "❯ No, do you trust this folder?\n● Yes, it asks whether you trust this folder.\n\n❯ ", false, false},
		{"transcript word that only starts with no", "❯ now fix the trust this folder bug\n  Yes, I trust this folder", false, false},
		{"input row with text", "● Earlier: do you trust this folder\n❯ no thanks\n", false, false},
		{"single highlighted answer without sibling", "Do you trust this folder?\n ❯ No, exit", false, false},
	}
	for _, c := range cases {
		yes, found := TrustSelection(c.screen)
		if yes != c.yes || found != c.found {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", c.name, yes, found, c.yes, c.found)
		}
	}
}
