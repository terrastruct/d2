package d2ir

import (
	"strings"

	"oss.terrastruct.com/d2/d2graph"
)

func (m *Map) doubleGlob(pattern []string) ([]*Field, bool) {
	if !(len(pattern) == 3 && pattern[0] == "*" && pattern[1] == "" && pattern[2] == "*") {
		return nil, false
	}
	var fa []*Field
	m._doubleGlob(&fa)
	return fa, true
}

func (m *Map) _doubleGlob(fa *[]*Field) {
	for _, f := range m.Fields {
		if _, ok := d2graph.ReservedKeywords[f.Name]; ok {
			continue
		}
		*fa = append(*fa, f)
		if f.Map() != nil {
			f.Map()._doubleGlob(fa)
		}
	}
}

func matchPattern(s string, pattern []string) bool {
	if len(pattern) == 0 {
		return true
	}
	if _, ok := d2graph.ReservedKeywords[s]; ok {
		return false
	}

	for i := 0; i < len(pattern); i++ {
		if pattern[i] == "*" {
			// * so match next.
			if i != len(pattern)-1 {
				j := strings.Index(strings.ToLower(s), strings.ToLower(pattern[i+1]))
				if j == -1 {
					return false
				}
				s = s[j+len(pattern[i+1]):]
				i++
			}
		} else {
			if !strings.HasPrefix(strings.ToLower(s), strings.ToLower(pattern[i])) {
				return false
			}
			s = s[len(pattern[i]):]
		}
	}
	return true
}
