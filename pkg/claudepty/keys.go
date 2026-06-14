package claudepty

import (
	"strconv"
	"strings"
)

// UnescapeKeys turns Go-style escape sequences in s into their
// corresponding bytes. Useful when keystrokes arrive as JSON-shaped
// strings (e.g. {"text": "\\r"} where the JSON literally contains
// backslash-r and the caller means "carriage return").
//
// Anything that doesn't parse as a Go literal is returned unchanged
// (assume the caller meant the literal text).
func UnescapeKeys(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	// strconv.Unquote requires surrounding quotes and handles all the
	// usual escapes (\r, \n, \t, \xNN, \uNNNN, \\, \"). Wrap and try.
	if unq, err := strconv.Unquote("\"" + escapeForUnquote(s) + "\""); err == nil {
		return unq
	}
	return s
}

func escapeForUnquote(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			b.WriteString(`\"`)
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
