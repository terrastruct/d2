package trees

import (
	"math"
	"slices"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

const (
	siblingSpacing   = 50.0
	directionPenalty = 2 * layoutgraph.TreeParentSpacing
)

// get a map from depth level 0-n to tree nodes at that depth
// .                     ┌───┐
// . L 0                 │   │
// .                     └───┘
// .        ┌───┐        ┌───┐        ┌───┐
// . L 1    │   │        │   │        │   │
// .        └───┘        └───┘        └───┘
// .     ┌───┐ ┌───┐  ┌───┐ ┌───┐  ┌───┐ ┌───┐
// . L 2 │   │ │   │  │   │ │   │  │   │ │   │
// .     └───┘ └───┘  └───┘ └───┘  └───┘ └───┘
func levelsByDepth(t *layoutgraph.Tree, guard *limits.WorkGuard) (map[int][]*layoutgraph.Tree, error) {
	if t == nil {
		return nil, treePreprocessBadState("tree placement cannot traverse a nil tree")
	}
	type leveledTree struct {
		tree  *layoutgraph.Tree
		level int
	}
	queue := []leveledTree{{tree: t}}
	seen := make(map[*layoutgraph.Tree]struct{})
	treeLevels := make(map[int][]*layoutgraph.Tree)
	for index := 0; index < len(queue); index++ {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		current := queue[index]
		if current.tree == nil || current.tree.Node == nil {
			return nil, treePreprocessBadState("tree placement encountered an incomplete tree")
		}
		if _, exists := seen[current.tree]; exists {
			return nil, treePreprocessBadState("tree placement encountered a repeated tree node")
		}
		seen[current.tree] = struct{}{}
		treeLevels[current.level] = append(treeLevels[current.level], current.tree)
		for _, child := range current.tree.Children {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			queue = append(queue, leveledTree{tree: child, level: current.level + 1})
		}
	}
	return treeLevels, nil
}

func shiftSubtreeHorizontally(t *layoutgraph.Tree, delta float64, guard *limits.WorkGuard) error {
	return offsetSubtree(t, delta, 0, guard)
}

func spacingToChildren(t *layoutgraph.Tree, guard *limits.WorkGuard) (float64, error) {
	maxLabelSize := 0.0
	for _, c := range t.Children {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if c == nil || c.SentinelEdge == nil {
			return 0, treePreprocessBadState("tree placement encountered an incomplete child edge")
		}
		labelSize := float64(c.SentinelEdge.MinHeight)
		if t.Orientation == geo.Left || t.Orientation == geo.Right {
			labelSize = float64(c.SentinelEdge.MinWidth)
		}
		maxLabelSize = math.Max(maxLabelSize, labelSize)
	}
	if maxLabelSize == 0 {
		return layoutgraph.TreeParentSpacing, nil
	}
	labelSpacingWithArrowheads := maxLabelSize + 2*layoutgraph.MinArrowheadClearance
	if len(t.Children) == 1 {
		//  ┌───┐
		//  │   │ We want enough space to place the label at the X
		//  └─┬─┘  ┬ MinArrowheadClearance
		//    │    ┼
		//   X│    │ maxLabelSize
		//    │    ┼
		//  ┌─▼─┐  ┴ MinArrowheadClearance
		//  │   │
		//  └───┘
		return math.Max(layoutgraph.TreeParentSpacing, labelSpacingWithArrowheads), nil
	}
	//         ┌───┐
	//         │   │       We want enough space to place the labels at the X positions
	//         └─┬─┘        ┬
	//           │          │ TreeParentSpacing/2
	//    ┌──────┴──────┐   ┼
	//    │             │   │
	//   X│             │X  │ labelSpacingWithArrowheads
	//    │             │   │
	//  ┌─▼─┐         ┌─▼─┐ ┴
	//  │   │         │   │
	//  └───┘         └───┘
	return layoutgraph.TreeParentSpacing/2 + math.Max(layoutgraph.TreeParentSpacing/2, labelSpacingWithArrowheads), nil
}

func layoutTree(t *layoutgraph.Tree, guard *limits.WorkGuard) error {
	treeLevels, err := levelsByDepth(t, guard)
	if err != nil {
		return err
	}

	// 1. The level height is the tallest node's at each level
	levelHeights := make(map[int]float64)
	for level := 0; level < len(treeLevels); level++ {
		levelHeight := 0.0
		for _, levelNode := range treeLevels[level] {
			if err := guard.Step(); err != nil {
				return err
			}
			levelHeight = math.Max(levelHeight, levelNode.Node.Height)
		}
		levelHeights[level] = levelHeight
	}
	// 1.1. Edge labels may require additional spacing, so spacing for the level is based on the largest label
	levelSpacings := make(map[int]float64)
	for level := 0; level < len(treeLevels)-1; level++ {
		for _, levelNode := range treeLevels[level] {
			if err := guard.Step(); err != nil {
				return err
			}
			spacing, err := spacingToChildren(levelNode, guard)
			if err != nil {
				return err
			}
			levelSpacings[level+1] = math.Max(levelSpacings[level+1], spacing)
		}
	}

	//   ┌───┐        ┌─────┐              ┌──┐ ┌───┐
	//   │   │ ┌───┐  │     │ ┌─────────┐  │  │ │   │
	// ------------------------------------------------
	//   │   │ └───┘  │     │ └─────────┘  │  │ │   │
	//   └───┘        └─────┘              └──┘ └───┘
	// 2. On each level, center all nodes to the center y value for that level
	levelBottom := 0.0
	for level := 1; level < len(treeLevels); level++ {
		levelTop := levelBottom + levelSpacings[level]
		for _, levelNode := range treeLevels[level] {
			if err := guard.Step(); err != nil {
				return err
			}
			centerOffset := math.Round((levelHeights[level] - levelNode.Node.Height) / 2)
			if err := moveTreeNodeAbsWithChildren(levelNode.Node, levelNode.Node.TopLeft.X, levelTop+centerOffset, guard); err != nil {
				return err
			}
		}
		levelBottom = levelTop + levelHeights[level]
	}

	// 3. Position all level nodes x values on each level, going bottom up by level
	for level := len(treeLevels) - 1; level > 0; level-- {
		// nextPosition moves along the x axis as we position each level node on this level
		nextPosition := 0.0
		for _, levelNode := range treeLevels[level] {
			if err := guard.Step(); err != nil {
				return err
			}
			if len(levelNode.Children) == 0 {
				// 3a. Simply set position according to nextPosition value
				if err := moveTreeNodeAbsWithChildren(levelNode.Node, nextPosition, levelNode.Node.TopLeft.Y, guard); err != nil {
					return err
				}
			} else {
				lastChildPosition := len(treeLevels[level+1])
				for i := lastChildPosition - 1; i >= 0; i-- {
					if err := guard.Step(); err != nil {
						return err
					}
					if treeLevels[level+1][i].Parent == levelNode {
						lastChildPosition = i
						break
					}
				}
				shiftSubsequentLevelNodes := func(dx float64) error {
					for i := lastChildPosition + 1; i < len(treeLevels[level+1]); i++ {
						if err := guard.Step(); err != nil {
							return err
						}
						if err := shiftSubtreeHorizontally(treeLevels[level+1][i], dx, guard); err != nil {
							return err
						}
					}
					return nil
				}

				// 3b 1. Space children evenly on the level below
				spacingBefore, err := totalChildrenSpacing(levelNode, guard)
				if err != nil {
					return err
				}
				if err := spaceChildrenEvenly(levelNode, guard); err != nil {
					return err
				}
				spacingAfter, err := totalChildrenSpacing(levelNode, guard)
				if err != nil {
					return err
				}
				totalShift := spacingAfter - spacingBefore
				// 3b 1a. if this shifts the children we also need to shift their subsequent level nodes
				if totalShift > 0 {
					if err := shiftSubsequentLevelNodes(totalShift); err != nil {
						return err
					}
				}
				// 3b 2. Center over children
				siblingCenter, err := childrenCenterX(levelNode, guard)
				if err != nil {
					return err
				}
				if err := moveTreeNodeAbsWithChildren(levelNode.Node, math.Round(siblingCenter-levelNode.Node.Width/2), levelNode.Node.TopLeft.Y, guard); err != nil {
					return err
				}

				// 3b 3. if after centering, this node's position is less than the desired spacing,
				// shift its whole subtree to reach the desired spacing (keeping the centering intact)
				positionDiff := nextPosition - levelNode.Node.TopLeft.X
				if positionDiff > 0 {
					if err := shiftSubtreeHorizontally(levelNode, positionDiff, guard); err != nil {
						return err
					}
					if err := shiftSubsequentLevelNodes(positionDiff); err != nil {
						return err
					}
				}
			}

			// 3c. the next position on this level is siblingSpacing past the end of this level node
			nextPosition = levelNode.Node.TopLeft.X + levelNode.Node.Width + siblingSpacing
		}
	}
	return guard.Finish()
}

func validateBottomOrientation(t *layoutgraph.Tree, guard *limits.WorkGuard) error {
	stack := []*layoutgraph.Tree{t}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil || current.Node.TopLeft == nil {
			return treePreprocessBadState("tree placement encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return treePreprocessBadState("tree placement encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		if current.Parent != nil {
			if current.Parent.Node == nil || current.Parent.Node.TopLeft == nil {
				return treePreprocessBadState("tree placement encountered an incomplete parent")
			}
			if current.Node.TopLeft.Y < current.Parent.Node.TopLeft.Y+current.Parent.Node.Height {
				return invariant.Errorf("tree node %d is not in Bottom orientation", current.Node.ID)
			}
		}
		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return err
			}
			stack = append(stack, v)
		}
	}
	return guard.Check()
}

func positionEdgeLabels(t *layoutgraph.Tree, guard *limits.WorkGuard) error {
	if t == nil {
		return treePreprocessBadState("tree placement cannot label a nil tree")
	}
	type labelTask struct {
		tree *layoutgraph.Tree
		emit bool
	}
	stack := []labelTask{{tree: t}}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		task := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		current := task.tree
		if current == nil || current.Node == nil {
			return treePreprocessBadState("tree placement encountered an incomplete label tree")
		}
		if !task.emit {
			if _, exists := seen[current]; exists {
				return treePreprocessBadState("tree placement encountered a repeated tree node")
			}
			seen[current] = struct{}{}
			stack = append(stack, labelTask{tree: current, emit: true})
			for _, v := range slices.Backward(current.Children) {
				if err := guard.Step(); err != nil {
					return err
				}
				stack = append(stack, labelTask{tree: v})
			}
			continue
		}

		edge := current.SentinelEdge
		if edge == nil {
			return treePreprocessBadState("tree node %d has no sentinel edge for label placement", current.Node.ID)
		}
		if edge.Label == nil {
			continue
		}
		labelSize := float64(edge.MinHeight)
		if current.Orientation.IsHorizontal() {
			labelSize = float64(edge.MinWidth)
		}
		if labelSize != 0 {
			if current.Parent == nil || current.Parent.Node == nil {
				return treePreprocessBadState("tree node %d has no parent for label placement", current.Node.ID)
			}
			if len(current.Parent.Children) == 1 {
				edge.Label.Position = label.UnlockedMiddle
				edge.LabelPercentage = 0.5
			} else {
				//         ┌───┐
				//  parent │   │       We want to place the labels at the X positions
				//         └─┬─┘        ┬    ┬
				//           │          │    │ TreeParentSpacing/2
				//    ┌──────┴──────┐   │    ┼
				//    │      .      │   │dy  │
				//   X│      .      │X  │    │ childSegmentLength
				//    │      .      │   │    │
				//  ┌─▼─┐    .    ┌─▼─┐ ┴    ┴
				//  │   │ child   │   │ child
				//  └───┘    .    └───┘
				//           ├──────┤
				//              dx
				childNode := current.Node
				parentNode := current.Parent.Node
				dx := childNode.Center().X - parentNode.Center().X
				dy := childNode.TopLeft.Y - (parentNode.TopLeft.Y + parentNode.Height)
				totalLength := math.Abs(dx) + dy
				childSegmentLength := dy - layoutgraph.TreeParentSpacing/2

				// LabelPercentage = distance from edge.From along edge / total edge distance
				distanceAlongEdge := childSegmentLength / 2
				isChildToParent := edge.From == childNode
				if !isChildToParent {
					distanceAlongEdge = totalLength - childSegmentLength/2
				}
				edge.LabelPercentage = distanceAlongEdge / totalLength
				// if the child is to the left, to be on the outside we want the label on the relative bottom position
				if dx < 0 {
					edge.Label.Position = label.UnlockedBottom
				} else {
					edge.Label.Position = label.UnlockedTop
				}
				// if the edge is in the other direction we need to mirror to get the same position
				if isChildToParent {
					edge.Label.Position = edge.Label.Position.Mirrored()
				}
				if current.Orientation.IsHorizontal() {
					// mirroring is needed due to the effects of invertOrientationToBottom on a horizontal orientation
					edge.Label.Position = edge.Label.Position.Mirrored()
				}
			}
		}
		if err := guard.Finish(); err != nil {
			return err
		}
	}
	return nil
}

// We compute the children's center by averaging the their centers
func childrenCenterX(t *layoutgraph.Tree, guard *limits.WorkGuard) (float64, error) {
	if len(t.Children) == 0 {
		return 0, treePreprocessBadState("tree placement cannot center a leaf")
	}
	centerX := 0.0
	for _, child := range t.Children {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if child == nil || child.Node == nil || child.Node.TopLeft == nil {
			return 0, treePreprocessBadState("tree placement encountered an incomplete child")
		}
		centerX += child.Node.TopLeft.X + child.Node.Width/2
	}
	// note: assumes this is only called on nodes with children
	centerX /= float64(len(t.Children))
	return centerX, nil
}

func spaceChildrenEvenly(t *layoutgraph.Tree, guard *limits.WorkGuard) error {
	maxSpacing := siblingSpacing

	childrenSpacing := make([]float64, 1, len(t.Children))
	// 1. Determine the maximum spacing between siblings
	for i := 1; i < len(t.Children); i++ {
		if err := guard.Step(); err != nil {
			return err
		}
		s := t.Children[i].Node.TopLeft.X - (t.Children[i-1].Node.TopLeft.X + t.Children[i-1].Node.Width)
		maxSpacing = math.Max(maxSpacing, s)
		childrenSpacing = append(childrenSpacing, s)
	}

	// 1a. Check that spacing evenly doesn't require too much space
	for i := 1; i < len(t.Children); i++ {
		if err := guard.Step(); err != nil {
			return err
		}
		shiftAmount := maxSpacing - childrenSpacing[i]
		if shiftAmount == 0 {
			continue
		}
		spacingBefore, err := spacingBefore(t, i, guard)
		if err != nil {
			return err
		}
		if spacingBefore+shiftAmount > 10*siblingSpacing {
			// spacing evenly would introduce too much excess space, so just keep the current spacing as-is
			return nil
		}
		// the next child will have less space after shifting
		if i+1 < len(t.Children) {
			childrenSpacing[i+1] -= shiftAmount
		}
	}

	// 2. Space all siblings with the maximum spacing
	for i := 1; i < len(t.Children); i++ {
		if err := guard.Step(); err != nil {
			return err
		}
		shiftAmount := maxSpacing - childrenSpacing[i]
		if shiftAmount > 0 {
			if err := shiftSubtreeHorizontally(t.Children[i], shiftAmount, guard); err != nil {
				return err
			}
		}
	}
	return nil
}

func totalChildrenSpacing(t *layoutgraph.Tree, guard *limits.WorkGuard) (float64, error) {
	spacing := 0.0
	for i := 1; i < len(t.Children); i++ {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		spacing += t.Children[i].Node.TopLeft.X - (t.Children[i-1].Node.TopLeft.X + t.Children[i-1].Node.Width)
	}
	return spacing, nil
}

func spacingBefore(t *layoutgraph.Tree, childIndex int, guard *limits.WorkGuard) (float64, error) {
	// returns the minimum space between child at i and i-1 including all descendant levels
	// .        ┌───┐            0, 1
	// .        │ t │ Children: [A, X]
	// .        └▲─▲┘
	// .    ┌────┘ └────┐
	// .  ┌─┴─┐   ┌─────┴─────┐
	// .  │ A │   │     X     │
	// .  └─▲─┘   └─────▲─────┘
	// .    │ ├─3─┤     │
	// . ┌──┴──┐      ┌─┴─┐
	// . │  B  │      │ Y │
	// . └─────┘      └───┘
	// .       ├───6──┤      return min(3,6)
	rights, err := levelRights(t.Children[childIndex-1], guard)
	if err != nil {
		return 0, err
	}
	lefts, err := levelLefts(t.Children[childIndex], guard)
	if err != nil {
		return 0, err
	}

	minSpace := lefts[0] - rights[0]
	for level := 1; level < len(lefts) && level < len(rights); level++ {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		minSpace = math.Min(minSpace, lefts[level]-rights[level])
	}
	return minSpace, nil
}

// returns the leftmost positions for each level of descendants from this node
func levelLefts(t *layoutgraph.Tree, guard *limits.WorkGuard) ([]float64, error) {
	levels, err := levelsByDepth(t, guard)
	if err != nil {
		return nil, err
	}
	lefts := make([]float64, len(levels))
	for level := 0; level < len(levels); level++ {
		if len(levels[level]) == 0 || levels[level][0].Node.TopLeft == nil {
			return nil, treePreprocessBadState("tree placement encountered an incomplete level")
		}
		if err := guard.Step(); err != nil {
			return nil, err
		}
		lefts[level] = levels[level][0].Node.TopLeft.X
	}
	return lefts, nil
}

func levelRights(t *layoutgraph.Tree, guard *limits.WorkGuard) ([]float64, error) {
	levels, err := levelsByDepth(t, guard)
	if err != nil {
		return nil, err
	}
	rights := make([]float64, len(levels))
	for level := 0; level < len(levels); level++ {
		if len(levels[level]) == 0 {
			return nil, treePreprocessBadState("tree placement encountered an empty level")
		}
		last := levels[level][len(levels[level])-1]
		if last.Node.TopLeft == nil {
			return nil, treePreprocessBadState("tree placement encountered an incomplete level")
		}
		if err := guard.Step(); err != nil {
			return nil, err
		}
		rights[level] = last.Node.TopLeft.X + last.Node.Width
	}
	return rights, nil
}

func centerAlignChildren(t *layoutgraph.Tree, guard *limits.WorkGuard) error {
	// 1. the x axis offset is computed so the root's children are centered with the root.
	// The center x value of the children should be equal to the root center x when aligned,
	// so we check how much to move everything in order to correct that.
	childrenCenter, err := childrenCenterX(t, guard)
	if err != nil {
		return err
	}
	xOffset := math.Round((t.Node.TopLeft.X + t.Node.Width/2) - childrenCenter)

	// 2. shift all children to be in alignment
	for _, child := range t.Children {
		if err := guard.Step(); err != nil {
			return err
		}
		if err := offsetSubtree(child, xOffset, 0, guard); err != nil {
			return err
		}
	}
	return guard.Finish()
}

// treePlacementDescendants is the guarded equivalent of
// Graph.AllDescendantNodes for tree-placement movement. Push order mirrors
// that helper so both traversals produce the same mutation order.
func treePlacementDescendants(node *layoutgraph.Node, guard *limits.WorkGuard) ([]*layoutgraph.Node, error) {
	if node == nil || node.Graph == nil {
		return nil, treePreprocessBadState("tree placement cannot move a node without graph ownership")
	}
	g := node.Graph
	seen := map[*layoutgraph.Node]struct{}{node: {}}
	stack := make([]*layoutgraph.Node, 0)
	pushChildren := func(parent *layoutgraph.Node) error {
		if parent == nil {
			return nil
		}
		if sequence := g.Sequences[parent]; sequence != nil {
			for _, v := range slices.Backward(sequence.Nodes) {
				if err := guard.Step(); err != nil {
					return err
				}
				stack = append(stack, v)
			}
		}
		if parent.IsClusterVessel() {
			if cluster := g.Clusters[parent]; cluster != nil {
				for _, v := range slices.Backward(cluster.Nodes) {
					if err := guard.Step(); err != nil {
						return err
					}
					stack = append(stack, v)
				}
			}
		}
		if parent.IsContainer() {
			children := g.Containers[parent]
			for _, c := range slices.Backward(children) {
				if err := guard.Step(); err != nil {
					return err
				}
				stack = append(stack, c)
			}
		}
		return nil
	}
	if err := pushChildren(node); err != nil {
		return nil, err
	}
	descendants := make([]*layoutgraph.Node, 0, len(stack))
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil {
			return nil, treePreprocessBadState("tree placement encountered a nil graph descendant")
		}
		if _, exists := seen[current]; exists {
			continue
		}
		seen[current] = struct{}{}
		descendants = append(descendants, current)
		if err := pushChildren(current); err != nil {
			return nil, err
		}
	}
	return descendants, nil
}

func moveTreeNodeWithChildren(node *layoutgraph.Node, dx, dy float64, guard *limits.WorkGuard) error {
	if node == nil || node.TopLeft == nil {
		return treePreprocessBadState("tree placement cannot move an incomplete node")
	}
	if dx == 0 && dy == 0 {
		return guard.Check()
	}
	descendants, err := treePlacementDescendants(node, guard)
	if err != nil {
		return err
	}
	if err := guard.Step(); err != nil {
		return err
	}
	node.Translate(dx, dy)
	if err := guard.Finish(); err != nil {
		return err
	}
	for _, descendant := range descendants {
		if err := guard.Step(); err != nil {
			return err
		}
		if descendant.TopLeft == nil {
			return treePreprocessBadState("tree placement encountered a descendant without a position")
		}
		descendant.Translate(dx, dy)
		if err := guard.Finish(); err != nil {
			return err
		}
	}
	return nil
}

func moveTreeNodeAbsWithChildren(node *layoutgraph.Node, x, y float64, guard *limits.WorkGuard) error {
	if node == nil || node.TopLeft == nil {
		return treePreprocessBadState("tree placement cannot move an incomplete node")
	}
	if node.TopLeft.X == x && node.TopLeft.Y == y {
		return guard.Check()
	}
	return moveTreeNodeWithChildren(node, x-node.TopLeft.X, y-node.TopLeft.Y, guard)
}

func offsetSubtree(t *layoutgraph.Tree, xOffset, yOffset float64, guard *limits.WorkGuard) error {
	stack := []*layoutgraph.Tree{t}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil {
			return treePreprocessBadState("tree placement encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return treePreprocessBadState("tree placement encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		if err := moveTreeNodeWithChildren(current.Node, xOffset, yOffset, guard); err != nil {
			return err
		}
		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return err
			}
			stack = append(stack, v)
		}
	}
	return guard.Finish()
}

// .     ┌─────────┐                   ┌──────────┐
// .     │ a       │                 ┌─┤ c        │
// .     └────┬────┘         ┌─────┐ │ └──────────┘
// .   ┌──────┴──────┐       │  a  │ │
// . ┌─┴──┐       ┌──┴─┐     │     ├─┤
// . │ b  │       │ c  │ <=> │     │ │
// . │    │       │    │     └─────┘ │ ┌──────────┐
// . │    │       │    │             └─┤ b        │
// . └────┘       └────┘               └──────────┘
// this swaps the x and y coordinates as well as the width and height of all nodes in the tree
// this is so that we can layout a tree to the right with the same code to lay it out to the bottom
// by swapping dimensions before and after
func swapDimensions(t *layoutgraph.Tree, guard *limits.WorkGuard) error {
	stack := []*layoutgraph.Tree{t}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil || current.Node.TopLeft == nil {
			return treePreprocessBadState("tree placement encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return treePreprocessBadState("tree placement encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		if err := moveTreeNodeAbsWithChildren(current.Node, current.Node.TopLeft.Y, current.Node.TopLeft.X, guard); err != nil {
			return err
		}
		current.Node.Width, current.Node.Height = current.Node.Height, current.Node.Width
		if err := guard.Finish(); err != nil {
			return err
		}
		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return err
			}
			stack = append(stack, v)
		}
	}
	return nil
}

// .                       |
// .  ┌────┐       ┌────┐  |
// .  │ c  │       │ b  │  |
// .  │    │       │    │  |
// .  │    │       │    │  |
// .  └──┬─┘       └─┬──┘  |
// .     └─────┬─────┘     |
// .      ┌────┴────┐      |
// .      │    a    │      |
// .      └─────────┘      |
// . ----------------------|-----------------------
// .                       |      ┌─────────┐
// .          ^            |      │    a    │
// .          |            |      └────┬────┘
// .        flips ->       |    ┌──────┴──────┐
// .                       |  ┌─┴──┐       ┌──┴─┐
// .                       |  │ b  │       │ c  │
// .                       |  │    │       │    │
// .                       |  │    │       │    │
// .                       |  └────┘       └────┘
// this flips all nodes in the tree such that the top left is the mirror topleft accounting for the width and height
func flip(t *layoutgraph.Tree, guard *limits.WorkGuard) error {
	stack := []*layoutgraph.Tree{t}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil || current.Node.TopLeft == nil {
			return treePreprocessBadState("tree placement encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return treePreprocessBadState("tree placement encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		if err := moveTreeNodeAbsWithChildren(
			current.Node,
			-(current.Node.TopLeft.X + current.Node.Width),
			-(current.Node.TopLeft.Y + current.Node.Height),
			guard,
		); err != nil {
			return err
		}
		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return err
			}
			stack = append(stack, v)
		}
	}
	return guard.Finish()
}

func setOrientation(t *layoutgraph.Tree, o geo.Orientation, guard *limits.WorkGuard) error {
	stack := []*layoutgraph.Tree{t}
	seen := make(map[*layoutgraph.Tree]struct{})
	for len(stack) > 0 {
		if err := guard.Step(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == nil || current.Node == nil {
			return treePreprocessBadState("tree placement encountered an incomplete tree")
		}
		if _, exists := seen[current]; exists {
			return treePreprocessBadState("tree placement encountered a repeated tree node")
		}
		seen[current] = struct{}{}
		current.Orientation = o
		if err := guard.Finish(); err != nil {
			return err
		}
		for _, v := range slices.Backward(current.Children) {
			if err := guard.Step(); err != nil {
				return err
			}
			stack = append(stack, v)
		}
	}
	return nil
}
