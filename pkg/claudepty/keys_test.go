package claudepty

import "testing"

func TestUnescapeKeys(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no escapes", "hello world", "hello world"},
		{"backslash-r becomes CR", `hello\r`, "hello\r"},
		{"backslash-n becomes LF", `hello\n`, "hello\n"},
		{"backslash-t becomes TAB", `a\tb`, "a\tb"},
		{"hex escape", `\x03`, "\x03"},
		{"unicode escape", `é`, "é"},
		{"mixed", `/usage\r`, "/usage\r"},
		{"unescaped double quotes survive", `say "hi"\r`, "say \"hi\"\r"},
		{"unparseable falls through unchanged", `bare \`, `bare \`},
		{"already-cooked text", "actual\r\n", "actual\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := UnescapeKeys(c.in)
			if got != c.want {
				t.Errorf("UnescapeKeys(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
