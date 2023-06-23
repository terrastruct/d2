package d2ir

import (
	"strings"
)

func matchPattern(s string, pattern []string) bool {
	if len(pattern) == 0 {
		return true
	}

	for i := 0; i < len(pattern); i++ {
		if pattern[i] == "*" {
			// * so match next.
			if i != len(pattern)-1 {
				j := strings.Index(s, pattern[i+1])
				if j == -1 {
					return false
				}
				s = s[j+len(pattern[i+1]):]
				i++
			}
		} else {
			if !strings.HasPrefix(s, pattern[i]) {
				return false
			}
			s = s[len(pattern[i]):]
		}
	}
	return true
}
