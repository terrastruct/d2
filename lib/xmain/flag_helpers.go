// flag_helpers.go are private functions from pflag/flag.go
package xmain

import "strings"

func wrap(i, w int, s string) string {
	if w == 0 {
		return strings.Replace(s, "\n", "\n"+strings.Repeat(" ", i), -1)
	}
	wrap := w - i
	var r, l string
	if wrap < 24 {
		i = 16
		wrap = w - i
		r += "\n" + strings.Repeat(" ", i)
	}
	if wrap < 24 {
		return strings.Replace(s, "\n", r, -1)
	}
	slop := 5
	wrap = wrap - slop
	l, s = wrapN(wrap, slop, s)
	r = r + strings.Replace(l, "\n", "\n"+strings.Repeat(" ", i), -1)
	for s != "" {
		var t string
		t, s = wrapN(wrap, slop, s)
		r = r + "\n" + strings.Repeat(" ", i) + strings.Replace(t, "\n", "\n"+strings.Repeat(" ", i), -1)
	}
	return r
}

func wrapN(i, slop int, s string) (string, string) {
	if i+slop > len(s) {
		return s, ""
	}
	w := strings.LastIndexAny(s[:i], " \t\n")
	if w <= 0 {
		return s, ""
	}
	nlPos := strings.LastIndex(s[:i], "\n")
	if nlPos > 0 && nlPos < w {
		return s[:nlPos], s[nlPos+1:]
	}
	return s[:w], s[w+1:]
}
