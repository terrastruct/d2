package graphbounds

import (
	"math"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// WorkStepper is the aggregate accounting contract shared by callers. Bounds
// never create or reset a budget: every scan is charged to the caller's guard.
type WorkStepper interface {
	// Step charges one unit of bounds work.
	Step() error
	// Finish observes cancellation after the final bounds operation.
	Finish() error
}

func isExtreme(
	nodes layoutgraph.Nodes,
	node *layoutgraph.Node,
	disqualifies func(*layoutgraph.Node) bool,
	guard WorkStepper,
) (bool, error) {
	for _, other := range nodes {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if other == node || other.TopLeft == nil {
			continue
		}
		if disqualifies(other) {
			return false, nil
		}
	}
	return true, nil
}

// NodeBoundingBox computes the packing-compatible bounds for one placed node. When
// allNodes is non-nil, outside labels and icons reserve their exact boundary
// space and outside-label extremity scans consume the shared work budget.
func NodeBoundingBox(
	node *layoutgraph.Node,
	allNodes layoutgraph.Nodes,
	guard WorkStepper,
) (*geo.Point, *geo.Point, error) {
	if err := guard.Step(); err != nil {
		return nil, nil, err
	}
	if node == nil || node.TopLeft == nil {
		return nil, nil, invariant.New("BinPack bounding box contains an unplaced node")
	}
	topLeft := node.TopLeft.Copy()
	bottomRight := geo.NewPoint(math.Round(topLeft.X+node.Width), math.Round(topLeft.Y+node.Height))
	if dx, dy := node.ModifierElementAdjustments(); dx != 0 || dy != 0 {
		topLeft.Y -= dy
		bottomRight.X += dx
	}
	topLeft.X -= node.LoopOffsets[geo.Left]
	topLeft.Y -= node.LoopOffsets[geo.Top]
	bottomRight.X += node.LoopOffsets[geo.Right]
	bottomRight.Y += node.LoopOffsets[geo.Bottom]

	if node.Label != nil && node.Label.Position.IsOutside() && allNodes != nil {
		labelTopLeft := node.LabelTopLeft(node.Label.Position, node.Label.Width, node.Label.Height)
		boundaryPadding := float64(label.PADDING)
		outsidePadding := 2. * label.PADDING
		if labelTopLeft.X < topLeft.X {
			extreme, err := isExtreme(allNodes, node, func(other *layoutgraph.Node) bool {
				return other.TopLeft.X < node.TopLeft.X
			}, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if extreme {
				padding = boundaryPadding
			}
			topLeft.X = math.Floor(labelTopLeft.X - padding)
		}
		if labelTopLeft.Y < topLeft.Y {
			extreme, err := isExtreme(allNodes, node, func(other *layoutgraph.Node) bool {
				return other.TopLeft.Y < node.TopLeft.Y
			}, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if extreme {
				padding = boundaryPadding
			}
			topLeft.Y = math.Floor(labelTopLeft.Y - padding)
		}
		if labelTopLeft.X > bottomRight.X {
			extreme, err := isExtreme(allNodes, node, func(other *layoutgraph.Node) bool {
				return other.TopLeft.X+other.Width > node.TopLeft.X+node.Width
			}, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if extreme {
				padding = boundaryPadding
			}
			bottomRight.X = math.Ceil(labelTopLeft.X + node.Label.Width + padding)
		}
		if labelTopLeft.Y > bottomRight.Y {
			extreme, err := isExtreme(allNodes, node, func(other *layoutgraph.Node) bool {
				return other.TopLeft.Y+other.Height > node.TopLeft.Y+node.Height
			}, guard)
			if err != nil {
				return nil, nil, err
			}
			padding := outsidePadding
			if extreme {
				padding = boundaryPadding
			}
			bottomRight.Y = math.Ceil(labelTopLeft.Y + node.Label.Height + padding)
		}
	}
	if node.Icon != nil && !node.IsImage() && node.Icon.Position.IsOutside() && allNodes != nil {
		iconSize := float64(layoutgraph.MaxIconSize)
		iconTopLeft := node.LabelTopLeft(node.Icon.Position, iconSize, iconSize)
		outsidePadding := 2. * label.PADDING
		topLeft.X = math.Min(topLeft.X, math.Floor(iconTopLeft.X-outsidePadding))
		topLeft.Y = math.Min(topLeft.Y, math.Floor(iconTopLeft.Y-outsidePadding))
		bottomRight.X = math.Max(bottomRight.X, math.Ceil(iconTopLeft.X+iconSize+outsidePadding))
		bottomRight.Y = math.Max(bottomRight.Y, math.Ceil(iconTopLeft.Y+iconSize+outsidePadding))
	}
	return topLeft, bottomRight, nil
}

// BoundingBox computes the packing-compatible bounds for a node set.
func BoundingBox(nodes layoutgraph.Nodes, guard WorkStepper) (*geo.Point, *geo.Point, error) {
	if len(nodes) == 0 {
		return geo.NewPoint(math.Inf(-1), math.Inf(-1)), geo.NewPoint(math.Inf(1), math.Inf(1)), guard.Finish()
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, node := range nodes {
		topLeft, bottomRight, err := NodeBoundingBox(node, nodes, guard)
		if err != nil {
			return nil, nil, err
		}
		minX = math.Min(minX, topLeft.X)
		minY = math.Min(minY, topLeft.Y)
		maxX = math.Max(maxX, bottomRight.X)
		maxY = math.Max(maxY, bottomRight.Y)
	}
	if err := guard.Finish(); err != nil {
		return nil, nil, err
	}
	return geo.NewPoint(minX, minY), geo.NewPoint(maxX, maxY), nil
}

func containerLevel(node *layoutgraph.Node, guard WorkStepper) (int, error) {
	level := 0
	seen := make(map[*layoutgraph.Node]struct{})
	for current := node; current != nil; current = current.OwningContainer() {
		if err := guard.Step(); err != nil {
			return 0, err
		}
		if _, exists := seen[current]; exists {
			return 0, invariant.New("BinPack found a cycle in container ancestry")
		}
		seen[current] = struct{}{}
		level++
	}
	return level, nil
}

func fixedOrigin(nodes layoutgraph.Nodes, guard WorkStepper) (*geo.Point, error) {
	var container *layoutgraph.Node
	minimumLevel := math.MaxInt
	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		current := node.OwningContainer()
		if current == nil {
			container = nil
			break
		}
		level, err := containerLevel(current, guard)
		if err != nil {
			return nil, err
		}
		if level < minimumLevel {
			minimumLevel = level
			container = current
		}
	}
	for _, child := range nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if child.OwningContainer() != container {
			continue
		}
		if origin := child.FixedOrigin(); origin != nil {
			return origin, nil
		}
	}
	return nil, guard.Finish()
}

// FixedBoundingBox computes node-set bounds and substitutes the inherited fixed origin
// used by packing when one of the relevant children is pinned.
func FixedBoundingBox(nodes layoutgraph.Nodes, guard WorkStepper) (*geo.Point, *geo.Point, error) {
	topLeft, bottomRight, err := BoundingBox(nodes, guard)
	if err != nil {
		return nil, nil, err
	}
	origin, err := fixedOrigin(nodes, guard)
	if err != nil {
		return nil, nil, err
	}
	if origin != nil {
		topLeft = origin
	}
	return topLeft, bottomRight, guard.Finish()
}
