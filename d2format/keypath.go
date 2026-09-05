package d2format

import (
	"strings"

	"github.com/d2lang/d2/d2ast"
)

// FormatKeyPath formats the scalar values in a as a canonical key path, like
// Format(d2ast.MakeKeyPathString(a)), without building a temporary key path AST.
// Quoting in the input nodes is intentionally ignored, just as in MakeKeyPathString.
func FormatKeyPath(a []d2ast.String) string {
	p := printer{inKey: true}
	for i, s := range a {
		if i > 0 {
			p.sb.WriteByte('.')
		}
		value := s.ScalarString()
		// The exported specials set can be customized by callers.
		if plainKey(value) && !strings.ContainsAny(value, d2ast.UnquotedKeySpecials) {
			lower := strings.ToLower(value)
			if lower == "null" {
				p.sb.WriteString("'null'")
			} else if _, reserved := d2ast.ReservedKeywords[lower]; reserved {
				p.sb.WriteString(lower)
			} else {
				p.sb.WriteString(value)
			}
		} else {
			p.node(d2ast.RawString(value, true))
		}
	}
	return p.sb.String()
}

// Keep the fast path deliberately narrow. Other strings use the regular
// formatter's quoting, escaping, Unicode, and invalid UTF-8 handling.
func plainKey(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' {
			continue
		}
		return false
	}
	return true
}
