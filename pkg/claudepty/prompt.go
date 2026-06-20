package claudepty

import "strings"

// promptChar is the cursor glyph claude renders at the start of any
// interactive row — both the main input box and modal menu options.
// Distinguishing the two from a raw byte stream is unreliable; the
// VT-grid path in HasInputPrompt does it row-shape-aware.
const promptChar = "❯"

// ReadyForInput reports whether the rendered screen shows claude's main
// input box focused and accepting keystrokes. It is grounded in claude's
// own cursor placement rather than the variable placeholder text: when the
// box is live, claude parks the visible editing cursor on the row that
// starts with the "❯" prompt glyph (sitting on top of the dimmed "Try …"
// placeholder, if any). Menu rows like "❯ 1. Yes, I trust this folder" are
// excluded so the trust modal's preselected option never reads as ready.
//
// This deliberately ignores what the placeholder says or whether it lingers
// (it fades after ~1s on some setups but persists on others), so detection
// no longer depends on claude's example-prompt wording.
func ReadyForInput(scr *Screen) bool {
	if scr == nil || !scr.Cursor.Visible {
		return false
	}
	if scr.Cursor.Row < 0 || scr.Cursor.Row >= len(scr.Lines) {
		return false
	}
	rest, ok := promptRowRemainder(scr.Lines[scr.Cursor.Row])
	return ok && !looksLikeMenuOption(rest)
}

// HasInputPrompt is the text-only fallback for ReadyForInput, used when the
// captured cursor can't be trusted. It reports whether any row looks like the
// main input prompt: a "❯" followed by nothing or just a placeholder. Menu
// rows ("❯ 1. Yes …") are excluded — they have option text after the glyph.
func HasInputPrompt(screen string) bool {
	for _, line := range strings.Split(screen, "\n") {
		rest, ok := promptRowRemainder(line)
		if !ok || looksLikeMenuOption(rest) {
			continue
		}
		if rest == "" || strings.HasPrefix(rest, "Try ") {
			return true
		}
	}
	return false
}

// promptRowRemainder reports whether line begins with the "❯" prompt glyph
// and returns whatever follows it, trimmed. ok is false for non-prompt rows.
func promptRowRemainder(line string) (rest string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, promptChar) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, promptChar)), true
}

// looksLikeMenuOption reports whether s is a numbered menu choice ("1. Yes",
// "2. No, exit") rather than free input — i.e. leading digits then a dot.
func looksLikeMenuOption(s string) bool {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && i < len(s) && s[i] == '.'
}

// HasStylePicker reports whether the screen is claude's first-run text-style
// picker. Its highlighted theme row is pointed at by "❯", which ReadyForInput
// can't tell apart from the real input prompt (the option text is unnumbered),
// so the driver must dismiss it explicitly. Detected by the dark/light mode
// option pair, which only co-occur on this screen.
func HasStylePicker(screen string) bool {
	lower := strings.ToLower(screen)
	return strings.Contains(lower, "dark mode") && strings.Contains(lower, "light mode")
}
