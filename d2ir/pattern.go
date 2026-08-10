package d2ir

import (
	"strings"

	"github.com/d2lang/d2/d2ast"
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

// pathToField returns the source-ordered field path from m to target. A nil
// result means target is outside m's subtree.
func (m *Map) pathToField(target *Field) []*Field {
	var reverse []*Field
	for current := target; current != nil; {
		parent := ParentMap(current)
		if parent == nil {
			return nil
		}
		reverse = append(reverse, current)
		if parent == m {
			path := make([]*Field, len(reverse))
			for i := range reverse {
				path[len(reverse)-1-i] = reverse[i]
			}
			return path
		}
		current = ParentField(parent)
	}
	return nil
}

func (m *Map) directChildToward(target *Field) *Field {
	path := m.pathToField(target)
	if len(path) == 0 {
		return nil
	}
	return path[0]
}

// multiGlobMatchesToward returns the fields in the changed top-level branch
// toward target that the existing recursive glob traversal would append. A
// whole branch is replayed (rather than only the target path) so reference and
// filter ordering remains identical within the affected branch.
func (m *Map) multiGlobMatchesToward(target *Field, pattern []string) []*Field {
	path := m.pathToField(target)
	if len(path) == 0 {
		return nil
	}
	var matches []*Field
	if d2ast.IsDoubleGlob(pattern) {
		_doubleGlobField(path[0], &matches)
		return matches
	}
	if d2ast.IsTripleGlob(pattern) {
		_tripleGlobField(path[0], &matches)
		return matches
	}
	return nil
}

func _doubleGlobField(f *Field, matches *[]*Field) {
	if f == nil || f.Name == nil {
		return
	}
	name := f.Name.ScalarString()
	if _, reserved := d2ast.ReservedKeywords[name]; reserved && f.Name.IsUnquoted() {
		if _, board := d2ast.BoardKeywords[name]; board {
			return
		}
		if f.Map() != nil {
			f.Map()._doubleGlob(matches)
		}
		return
	}
	if NodeBoardKind(f) == "" {
		*matches = append(*matches, f)
	}
	if f.Map() != nil {
		f.Map()._doubleGlob(matches)
	}
}

func _tripleGlobField(f *Field, matches *[]*Field) {
	if f == nil || f.Name == nil {
		return
	}
	name := f.Name.ScalarString()
	if _, reserved := d2ast.ReservedKeywords[name]; reserved && f.Name.IsUnquoted() {
		if _, board := d2ast.BoardKeywords[name]; !board {
			return
		}
		if f.Map() != nil {
			f.Map()._tripleGlob(matches)
		}
		return
	}
	if NodeBoardKind(f) == "" {
		*matches = append(*matches, f)
	}
	if f.Map() != nil {
		f.Map()._tripleGlob(matches)
	}
}

func (m *Map) _doubleGlob(fa *[]*Field) {
	for _, f := range m.Fields {
		if f.Name == nil {
			continue
		}
		if _, ok := d2ast.ReservedKeywords[f.Name.ScalarString()]; ok && f.Name.IsUnquoted() {
			if _, ok := d2ast.BoardKeywords[f.Name.ScalarString()]; ok {
				continue
			}
			if f.Map() != nil {
				f.Map()._doubleGlob(fa)
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
			if i == len(pattern)-1 {
				return true
			}

			next := strings.ToLower(pattern[i+1])
			lowerS := strings.ToLower(s)
			if i+1 == len(pattern)-1 {
				return strings.HasSuffix(lowerS, next)
			}

			// Match the earliest occurrence so the remaining wildcards have the
			// largest possible suffix to search.
			j := strings.Index(lowerS, next)
			if j == -1 {
				return false
			}
			s = s[j+len(pattern[i+1]):]
			i++
		} else {
			if !strings.HasPrefix(strings.ToLower(s), strings.ToLower(pattern[i])) {
				return false
			}
			s = s[len(pattern[i]):]
		}
	}
	return s == ""
}
