package claudepty

import "strings"

// promptChar is the cursor glyph claude renders at the start of any
// interactive row — both the main input box and modal menu options.
// Distinguishing the two from a raw byte stream is unreliable; the
// VT-grid path in HasInputPrompt does it row-shape-aware.
const promptChar = "❯"

// HasInputPrompt reports whether the rendered grid contains a row that
// looks like claude's main input prompt: a "❯" followed by either
// nothing else or just a placeholder suggestion (Try "..."). Menu rows
// like "❯ 1. Yes, I trust this folder" don't match — they have option
// text after the glyph.
func HasInputPrompt(screen string) bool {
	for _, line := range strings.Split(screen, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == promptChar {
			return true
		}
		if strings.HasPrefix(trimmed, promptChar+" Try ") {
			return true
		}
	}
	return false
}
