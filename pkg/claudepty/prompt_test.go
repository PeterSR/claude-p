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
