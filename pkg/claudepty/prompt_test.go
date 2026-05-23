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
