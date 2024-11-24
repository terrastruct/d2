package d2ir

import (
	"strings"

	"oss.terrastruct.com/d2/d2ast"
)

func (m *Map) multiGlob(pattern []string) ([]*Field, bool) {
	var fa []*Field
	if d2ast.IsDoubleGlob(pattern) {
		m._doubleGlob(&fa)
		return fa, true
	}
	if d2ast.IsTripleGlob(pattern) {
		m._tripleGlob(&fa)
		return fa, true
	}
	return nil, false
}

func (m *Map) _doubleGlob(fa *[]*Field) {
	for _, f := range m.Fields {
		if f.Name == nil {
			continue
		}
		if _, ok := d2ast.ReservedKeywords[f.Name.ScalarString()]; ok && f.Name.IsUnquoted() {
			if f.Name.ScalarString() == "layers" {
				continue
			}
			if _, ok := d2ast.BoardKeywords[f.Name.ScalarString()]; !ok {
				continue
			}
			// We don't ever want to append layers, scenarios or steps directly.
			if f.Map() != nil {
				f.Map()._tripleGlob(fa)
			}
			continue
		}
		if NodeBoardKind(f) == "" {
			*fa = append(*fa, f)
		}
		if f.Map() != nil {
			f.Map()._doubleGlob(fa)
		}
	}
}

func (m *Map) _tripleGlob(fa *[]*Field) {
	for _, f := range m.Fields {
		if _, ok := d2ast.ReservedKeywords[f.Name.ScalarString()]; ok && f.Name.IsUnquoted() {
			if _, ok := d2ast.BoardKeywords[f.Name.ScalarString()]; !ok {
				continue
			}
			// We don't ever want to append layers, scenarios or steps directly.
			if f.Map() != nil {
				f.Map()._tripleGlob(fa)
			}
			continue
		}
		if NodeBoardKind(f) == "" {
			*fa = append(*fa, f)
		}
		if f.Map() != nil {
			f.Map()._tripleGlob(fa)
		}
	}
}

func matchPattern(s string, pattern []string) bool {
	if len(pattern) == 0 {
		return true
	}
	if _, ok := d2ast.ReservedKeywords[s]; ok {
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
