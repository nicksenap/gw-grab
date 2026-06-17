package source

import (
	"strings"
	"unicode"
)

// Slugify turns arbitrary text into a safe branch/workspace name fragment:
// lowercase, [a-z0-9-] only, collapsed and trimmed dashes, capped at maxLen on a
// word boundary. Returns "" if nothing usable remains (caller should fall back
// to a provider-prefixed name like "pr-1172").
func Slugify(s string, maxLen int) string {
	s = strings.ToLower(strings.TrimSpace(s))

	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r), r == '-', r == '_', r == '/', r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if maxLen > 0 && len(out) > maxLen {
		out = out[:maxLen]
		// Trim back to a word boundary so we don't cut mid-word.
		if i := strings.LastIndexByte(out, '-'); i > 0 {
			out = out[:i]
		}
		out = strings.Trim(out, "-")
	}
	return out
}
