package grouping

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
)

type joinNodePositionSnapshot struct {
	node    *layoutgraph.Node
	pointer *geo.Point
	value   geo.Point
}

// joinPositionJournal keeps capture order because separate nodes may share a
// TopLeft pointer and first reach the journal after different movements.
type joinPositionJournal struct {
	captured  map[*layoutgraph.Node]struct{}
	snapshots []joinNodePositionSnapshot
}

func (journal *joinPositionJournal) capture(node *layoutgraph.Node) bool {
	if node == nil {
		return false
	}
	if journal.captured == nil {
		journal.captured = make(map[*layoutgraph.Node]struct{})
	}
	if _, ok := journal.captured[node]; ok {
		return false
	}
	journal.captured[node] = struct{}{}
	snapshot := joinNodePositionSnapshot{node: node, pointer: node.TopLeft}
	if node.TopLeft != nil {
		snapshot.value = *node.TopLeft
	}
	journal.snapshots = append(journal.snapshots, snapshot)
	return true
}

func (journal *joinPositionJournal) moveWithChildren(
	node *layoutgraph.Node,
	dx, dy float64,
	guard *limits.WorkGuard,
) error {
	if dx == 0 && dy == 0 {
		return nil
	}
	firstCapture := journal.capture(node)
	if node.Graph == nil {
		if err := guard.Step(); err != nil {
			return err
		}
		// Preserve the legacy malformed-node panic after journaling the root.
		node.MoveWithChildren(dx, dy)
		return guard.Finish()
	}
	graph := node.Graph
	hasDescendants := node.IsContainer() && len(graph.Containers[node]) > 0
	if node.IsClusterVessel() {
		hasDescendants = hasDescendants || graph.Clusters[node] != nil && len(graph.Clusters[node].Nodes) > 0
	}
	hasDescendants = hasDescendants || graph.Sequences[node] != nil && len(graph.Sequences[node].Nodes) > 0
	var descendants []*layoutgraph.Node
	if hasDescendants {
		var err error
		descendants, err = graph.AllDescendantNodesWithWorkGuard(node, true, guard)
		if err != nil {
			return err
		}
	}
	if firstCapture {
		for _, descendant := range descendants {
			journal.capture(descendant)
		}
	}
	// Reject the complete translation before changing any shared point. The
	// guarded descendant walk above separately accounts hierarchy discovery.
	if err := guard.Add(int64(1 + len(descendants))); err != nil {
		return err
	}
	node.Translate(dx, dy)
	for _, descendant := range descendants {
		descendant.Translate(dx, dy)
	}
	return nil
}

func (journal *joinPositionJournal) restore() {
	// Reverse capture order restores shared pointers through every intermediate
	// value before returning them to their original value.
	for index := len(journal.snapshots) - 1; index >= 0; index-- {
		snapshot := journal.snapshots[index]
		snapshot.node.TopLeft = snapshot.pointer
		if snapshot.pointer != nil {
			*snapshot.pointer = snapshot.value
		}
	}
}

func joinMedian(points []*geo.Point, guard *limits.WorkGuard) (*geo.Point, error) {
	if err := guard.Add(2 * int64(len(points))); err != nil {
		return nil, err
	}
	for width := 1; width < len(points); width *= 2 {
		if err := guard.Add(2 * int64(len(points))); err != nil {
			return nil, err
		}
		if width > len(points)/2 {
			break
		}
	}
	return geo.Points(points).GetMedian(), nil
}

func joinContainerFixedOrigin(
	graph *layoutgraph.Graph,
	container *layoutgraph.Node,
	guard *limits.WorkGuard,
) (*geo.Point, error) {
	for _, node := range graph.Nodes {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		if node.OwningContainer() != container {
			continue
		}
		if origin := node.FixedOrigin(); origin != nil {
			return origin, nil
		}
	}
	return nil, nil
}

// JoinDistancedClusters incrementally pulls disconnected placement islands
// toward their common center without introducing overlap.
func JoinDistancedClusters(ctx context.Context, graph *layoutgraph.Graph) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("PlaceNodes: %w", err)
	}
	guard, sharedGuard, err := layoutgraph.ExistingTransactionWorkGuard(ctx, "PlaceNodes")
	if err != nil {
		return err
	}
	preflightWork := int64(len(graph.Nodes))
	if sharedGuard {
		if err := guard.Add(preflightWork); err != nil {
			return err
		}
	}
	fixedNodes := graph.FixedNodes()
	if len(graph.Nodes)-len(fixedNodes) <= 1 {
		if sharedGuard {
			return guard.Finish()
		}
		return nil
	}
	if len(graph.Edges) == 0 {
		preflightWork += int64(len(graph.Nodes))
		if sharedGuard {
			if err := guard.Add(int64(len(graph.Nodes))); err != nil {
				return err
			}
		}
		hasJoinReference := false
		for _, node := range graph.Nodes {
			if len(node.Edges) > 0 || len(node.Nears) > 0 {
				hasJoinReference = true
				break
			}
		}
		if !hasJoinReference {
			if sharedGuard {
				return guard.Finish()
			}
			return nil
		}
	}
	if !sharedGuard {
		guard, err = limits.NewWorkGuard(ctx, "PlaceNodes", limits.MaxTransactionWorkUnits)
		if err != nil {
			return err
		}
		if err := guard.Add(preflightWork); err != nil {
			return err
		}
	}
	var positions joinPositionJournal
	complete := false
	defer func() {
		if !complete {
			positions.restore()
		}
	}()
	finish := func() error {
		if err := guard.Finish(); err != nil {
			return err
		}
		complete = true
		return nil
	}
	clusters, err := layoutgraph.Nodes(graph.Nodes).DistanceClustersWithWorkGuard(3*graph.CellSize, guard)
	if err != nil {
		return err
	}
	if clusters == nil {
		return finish()
	}

	var target *geo.Point
	if len(fixedNodes) > 0 {
		target, err = layoutgraph.Nodes(fixedNodes).CenterWithWorkGuard(guard)
		if err != nil {
			return err
		}
	} else {
		centers := make([]*geo.Point, 0, len(clusters))
		for _, cluster := range clusters {
			center, err := layoutgraph.Nodes(cluster).CenterWithWorkGuard(guard)
			if err != nil {
				return err
			}
			centers = append(centers, center)
		}
		target, err = joinMedian(centers, guard)
		if err != nil {
			return err
		}
	}

	inchTowardsTarget := func(cluster []*layoutgraph.Node, fixedOrigin *geo.Point) (bool, error) {
		topLeft, bottomRight, err := layoutgraph.Nodes(cluster).BoundingBoxWithWorkGuard(guard)
		if err != nil {
			return false, err
		}
		center := geo.NewPoint(topLeft.X+(bottomRight.X-topLeft.X)/2, topLeft.Y+(bottomRight.Y-topLeft.Y)/2)
		distance := geo.EuclideanDistance(center.X, center.Y, target.X, target.Y)
		length := math.Max(bottomRight.X-topLeft.X, bottomRight.Y-topLeft.Y)
		if distance < length/2 {
			return false, nil
		}
		var deltaX, deltaY float64
		orientation := center.GetOrientation(target)
		if orientation == geo.NONE {
			return false, invariant.New("No orientation")
		}
		if orientation == geo.Top || orientation == geo.TopLeft || orientation == geo.TopRight {
			deltaY = 1
		}
		if orientation == geo.Bottom || orientation == geo.BottomLeft || orientation == geo.BottomRight {
			deltaY = -1
		}
		if orientation == geo.Left || orientation == geo.BottomLeft || orientation == geo.TopLeft {
			deltaX = 1
		}
		if orientation == geo.Right || orientation == geo.TopRight || orientation == geo.BottomRight {
			deltaX = -1
		}
		if deltaX == 0 && deltaY == 0 {
			return false, nil
		}
		deltaX *= graph.CellSize
		deltaY *= graph.CellSize

		hasOverlap, hasOverlapX, hasOverlapY := false, false, false
		for _, node := range cluster {
			x, y := node.TopLeft.X+deltaX, node.TopLeft.Y+deltaY
			if !hasOverlap {
				if fixedOrigin != nil {
					if x < fixedOrigin.X {
						hasOverlapX, hasOverlap = true, true
					}
					if y < fixedOrigin.Y {
						hasOverlapY, hasOverlap = true, true
					}
				}
				if !hasOverlap {
					var err error
					hasOverlap, err = graph.WouldOverlapWithWorkGuard(node, geo.NewPoint(x, y), cluster, nil, guard)
					if err != nil {
						return false, err
					}
				}
			}
			if err := positions.moveWithChildren(node, deltaX, deltaY, guard); err != nil {
				return false, err
			}
		}
		if hasOverlap {
			for _, node := range cluster {
				if err := positions.moveWithChildren(node, -deltaX, -deltaY, guard); err != nil {
					return false, err
				}
			}
		}
		if deltaX != 0 && deltaY != 0 {
			if hasOverlapX && !hasOverlapY {
				hasOverlap = false
				for _, node := range cluster {
					x, y := node.TopLeft.X, node.TopLeft.Y+deltaY
					overlaps := fixedOrigin != nil && (x < fixedOrigin.X || y < fixedOrigin.Y)
					if !overlaps {
						var err error
						overlaps, err = graph.WouldOverlapWithWorkGuard(node, geo.NewPoint(x, y), cluster, nil, guard)
						if err != nil {
							return false, err
						}
					}
					if overlaps {
						hasOverlap = true
					}
					if err := positions.moveWithChildren(node, 0, deltaY, guard); err != nil {
						return false, err
					}
				}
				if hasOverlap {
					for _, node := range cluster {
						if err := positions.moveWithChildren(node, 0, -deltaY, guard); err != nil {
							return false, err
						}
					}
				}
			} else if !hasOverlapX && hasOverlapY {
				hasOverlap = false
				for _, node := range cluster {
					x, y := node.TopLeft.X+deltaX, node.TopLeft.Y
					overlaps := fixedOrigin != nil && (x < fixedOrigin.X || y < fixedOrigin.Y)
					if !overlaps {
						var err error
						overlaps, err = graph.WouldOverlapWithWorkGuard(node, geo.NewPoint(x, y), cluster, nil, guard)
						if err != nil {
							return false, err
						}
					}
					if overlaps {
						hasOverlap = true
					}
					if err := positions.moveWithChildren(node, deltaX, 0, guard); err != nil {
						return false, err
					}
				}
				if hasOverlap {
					for _, node := range cluster {
						if err := positions.moveWithChildren(node, -deltaX, 0, guard); err != nil {
							return false, err
						}
					}
				}
			}
		}
		return !hasOverlap, nil
	}

	for range 1000 {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("PlaceNodes: %w", err)
		}
		if err := guard.Add(int64(1 + len(clusters))); err != nil {
			return err
		}
		moved := false
		for _, cluster := range clusters {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("PlaceNodes: %w", err)
			}
			var fixedOrigin *geo.Point
			if len(fixedNodes) > 0 {
				fixedOrigin, err = joinContainerFixedOrigin(graph, cluster[0].OwningContainer(), guard)
				if err != nil {
					return err
				}
			}
			clusterMoved, err := inchTowardsTarget(cluster, fixedOrigin)
			if err != nil {
				return err
			}
			if clusterMoved {
				moved = true
			}
		}
		if !moved {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("PlaceNodes: %w", err)
	}
	return finish()
}
