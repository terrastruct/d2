package hierarchy

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

func newHierarchyWithLevels(levels map[*layoutgraph.Node]int) *layoutgraph.Hierarchy {
	hierarchy := layoutgraph.NewHierarchy()
	hierarchy.ReplaceLevels(levels)
	return hierarchy
}
