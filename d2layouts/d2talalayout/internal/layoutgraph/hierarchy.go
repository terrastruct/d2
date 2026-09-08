package layoutgraph

// Hierarchy records the level assigned to each member of a directed, layered
// subgraph together with the discovery facts used by later layout stages.
// Discovery and placement live in the sibling hierarchy package; this type
// remains here because nodes own their hierarchy membership as graph state.
type Hierarchy struct {
	level map[*Node]int

	LevelCount int
}

// NewHierarchy creates an empty hierarchy record.
func NewHierarchy() *Hierarchy {
	return &Hierarchy{level: make(map[*Node]int)}
}

// Levels returns the hierarchy's node-to-level mapping. The hierarchy
// algorithms mutate this map while assigning and replacing memberships.
func (h *Hierarchy) Levels() map[*Node]int {
	if h.level == nil {
		h.level = make(map[*Node]int)
	}
	return h.level
}

// ReplaceLevels atomically replaces the node-to-level mapping.
func (h *Hierarchy) ReplaceLevels(levels map[*Node]int) {
	h.level = levels
}

// HierarchyLevel returns n's assigned level. Callers must only use it for a
// node with non-nil Hierarchy membership.
func (n *Node) HierarchyLevel() int {
	return n.Hierarchy.level[n]
}
