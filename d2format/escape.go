package d2format

import (
	"strings"

	"oss.terrastruct.com/d2/d2ast"
)

func escapeSingleQuotedValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\'':
			b.WriteByte('\'')
		case '\n':
			// TODO: Unified string syntax.
			b.WriteByte('\\')
			b.WriteByte('n')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func escapeDoubledQuotedValue(s string, inKey bool) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
		case '\n':
			b.WriteByte('\\')
			b.WriteByte('n')
			continue
		}
		if !inKey && r == '$' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func escapeUnquotedValue(s string, inKey bool) string {
	if len(s) == 0 {
		return `""`
	}

	if strings.EqualFold(s, "null") {
		return "\\null"
	}

	var b strings.Builder
	for i, r := range s {
		switch r {
		case '\'', '"', '|':
			if i == 0 {
				b.WriteByte('\\')
			}
		case '\n':
			b.WriteByte('\\')
			b.WriteByte('n')
			continue
		default:
			if inKey {
				switch r {
				case '-':
					if i+1 < len(s) && s[i+1] == '-' {
						b.WriteByte('\\')
					}
				case '&':
					if i == 0 {
						b.WriteByte('\\')
					}
				default:
					if strings.ContainsRune(d2ast.UnquotedKeySpecials, r) {
						b.WriteByte('\\')
					}
				}
			} else if strings.ContainsRune(d2ast.UnquotedValueSpecials, r) {
				b.WriteByte('\\')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}
