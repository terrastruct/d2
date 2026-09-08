package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/shape"
)

// used only in tests to check if the graph remains the same after Autolayout
//
//nolint:deadcode,unused
func checkGraphPropertiesAfterAutolayout(start, afterAutolayout *layoutgraph.Graph) error {
	if err := checkStructuralGraphPropertiesAfterAutolayout(start, afterAutolayout); err != nil {
		return err
	}
	if err := checkContainersSizes(afterAutolayout); err != nil {
		return err
	}
	if err := checkOverlap(afterAutolayout); err != nil {
		return err
	}
	return nil
}

// checkStructuralGraphPropertiesAfterAutolayout verifies semantic invariants.
// Layout-quality assertions, such as how tightly children fill a container,
// belong to golden and benchmark tests rather than fuzz safety checks.
func checkStructuralGraphPropertiesAfterAutolayout(start, afterAutolayout *layoutgraph.Graph) error {
	sequenceDefiningEdges, err := grouping.SequenceDefiningEdges(context.Background(), start)
	if err != nil {
		return err
	}
	if err := checkNodes(start, afterAutolayout, sequenceDefiningEdges); err != nil {
		return err
	}
	if err := checkEdges(start.Edges, afterAutolayout.Edges, sequenceDefiningEdges); err != nil {
		return err
	}
	if err := checkContainers(start, afterAutolayout); err != nil {
		return err
	}
	return nil
}

//nolint:unused
func checkNodes(start, result *layoutgraph.Graph, sequenceDefiningEdges map[layoutgraph.EntityID]struct{}) error {
	if len(start.Nodes) != len(result.Nodes) {
		return fmt.Errorf("node count changed from %d to %d", len(start.Nodes), len(result.Nodes))
	}

	startNodes := make([]*layoutgraph.Node, len(start.Nodes))
	copy(startNodes, start.Nodes)
	resultNodes := make([]*layoutgraph.Node, len(result.Nodes))
	copy(resultNodes, result.Nodes)

	sort.Slice(startNodes, func(i, j int) bool {
		return startNodes[i].ID < startNodes[j].ID
	})
	sort.Slice(resultNodes, func(i, j int) bool {
		return resultNodes[i].ID < resultNodes[j].ID
	})

	for i := 0; i < len(startNodes); i++ {
		s := startNodes[i]
		r := resultNodes[i]
		if s.ID != r.ID {
			return fmt.Errorf("node order changed at %d, %d != %d", i, s.ID, r.ID)
		}
		if err := checkEdges(s.Edges, r.Edges, sequenceDefiningEdges); err != nil {
			return err
		}
		if r.AspectRatio1() && r.Width != r.Height {
			return fmt.Errorf(
				"Node %d has shape %s that should stay square but width %.1f != height %.1f",
				r.ID,
				r.ShapeType(),
				r.Width,
				r.Height,
			)
		}
	}
	return nil
}

//nolint:unused
func checkEdges(startEdges, result []*layoutgraph.Edge, isSequenceDefiningEdges map[layoutgraph.EntityID]struct{}) error {
	isResultEdges := make(map[layoutgraph.EntityID]*layoutgraph.Edge)
	for _, e := range result {
		isResultEdges[e.ID] = e
	}

	for _, s := range startEdges {
		r, exists := isResultEdges[s.ID]
		if !exists {
			if _, isSequenceEdge := isSequenceDefiningEdges[s.ID]; isSequenceEdge {
				// if missing and isSequenceEdge, then it is fine
				continue
			}
			return fmt.Errorf("edge %d is missing", s.ID)
		}
		if s.From.ID != r.From.ID {
			return fmt.Errorf("edge.From changed at %d, %d != %d", s.ID, s.From.ID, r.From.ID)
		}
		if s.To.ID != r.To.ID {
			return fmt.Errorf("edge.To changed at %d, %d != %d", s.ID, s.To.ID, r.To.ID)
		}
		delete(isResultEdges, s.ID)
	}

	if len(isResultEdges) != 0 {
		var edgeIDs []string
		for edgeID := range isResultEdges {
			edgeIDs = append(edgeIDs, strconv.FormatInt(edgeID, 10))
		}
		return fmt.Errorf("the edges '%s' exist in the result graph but not in the start one", strings.Join(edgeIDs, ","))
	}

	return nil
}

//nolint:unused
func checkContainers(start, result *layoutgraph.Graph) error {
	if len(start.Containers) != len(result.Containers) {
		return fmt.Errorf("containers changed from %d to %d", len(start.Containers), len(result.Containers))
	}

	for startContainer, startChildren := range start.Containers {
		var resultContainer *layoutgraph.Node = nil
		if startContainer != nil {
			resultContainer = nodeByID(result, startContainer.ID)
			if resultContainer == nil {
				return fmt.Errorf("start container %d mapped to nil container in result", startContainer.ID)
			}
		}
		if _, isContainer := result.Containers[resultContainer]; !isContainer {
			return fmt.Errorf("start node %d is container, but is not in result", startContainer.ID)
		}
		if len(start.Containers[startContainer]) != len(result.Containers[resultContainer]) {
			return fmt.Errorf(
				"children changed from container %d, %d != %d",
				startContainer.ID,
				len(start.Containers[startContainer]),
				len(result.Containers[resultContainer]),
			)
		}
		for _, startChild := range startChildren {
			resultChild := nodeByID(result, startChild.ID)
			if startChild.IsDescendantOf(startContainer) != resultChild.IsDescendantOf(resultContainer) {
				return fmt.Errorf("container %d has child %d in start but not in result", startContainer.ID, startChild.ID)
			}
			if resultContainer != nil && !resultContainer.Covers(resultChild) {
				return fmt.Errorf("container %d does not cover child %d", resultContainer.ID, resultChild.ID)
			}
		}
	}
	return nil
}

//nolint:unused
func checkContainersSizes(result *layoutgraph.Graph) error {
	/* If the container must stay square, we allow some empty space.
	Take the example below

	Original
	┌───────────────┐
	│ ┌───┐   ┌───┐ │
	│ │   │   │   │ │
	│ │ a │   │ b │ │
	│ │   │   │   │ │
	│ └───┘   └───┘ │
	└───────────────┘

	After alignAxes, we move `b``, then we need to match the
	container height/width so it stays a perfect square and it'll create
	some empty space that fails the optimal placement/area usage above.
	┌─────────────────────────────────┐
	│ ┌───┐                     ┌───┐ │
	│ │   │                     │   │ │
	│ │ a │                     │ b │ │
	│ │   │                     │   │ │
	│ └───┘                     └───┘ │
	│                                 │
	│                                 │
	│                                 │
	│                                 │
	│                                 │
	│                                 │
	└─────────────────────────────────┘

	So in this case, if it's a perfect square and it only broke one of the sides, we allow it.

	Similarly, for clouds, we allow one side to be broken in case the children are aligned
	to the bottom and then there's some substantial distance to the top of the cloud.
	*/
	for container, children := range result.Containers {
		if container == nil {
			continue
		}
		// checks if half the container is empty
		maxDx := container.Width / 2.0
		maxDy := container.Height / 2.0

		childrenTL, childrenBR := layoutgraph.Nodes(children).FixedBoundingBox()
		tl, br := container.BoundingBox(nil)

		var overflowSides []string
		if math.Abs(childrenTL.X-tl.X) > maxDx {
			overflowSides = append(overflowSides, "left")
		}
		if math.Abs(childrenBR.X-br.X) > maxDx {
			overflowSides = append(overflowSides, "right")
		}
		if math.Abs(childrenTL.Y-tl.Y) > maxDy {
			overflowSides = append(overflowSides, "top")
		}
		// Person/Document type can have some extra space at bottom
		if math.Abs(childrenBR.Y-br.Y) > maxDy && container.ShapeType() != shape.PERSON_TYPE && container.ShapeType() != shape.DOCUMENT_TYPE {
			overflowSides = append(overflowSides, "bottom")
		}

		var badContainerSize bool
		if container.ShapeType() == shape.CLOUD_TYPE {
			badContainerSize = len(overflowSides) > 1
		} else if container.AspectRatio1() {
			badContainerSize = len(overflowSides) > 2
		} else {
			badContainerSize = len(overflowSides) > 0
		}

		if badContainerSize {
			return fmt.Errorf(
				"container %d, of type %s, has overflow in inner padding on: %s",
				container.ID,
				container.ShapeType(),
				strings.Join(overflowSides, ","),
			)
		}
	}
	return nil
}

//nolint:unused
func checkOverlap(result *layoutgraph.Graph) error {
	for i := 0; i < len(result.Nodes)-1; i++ {
		for j := i + 1; j < len(result.Nodes); j++ {
			ni := result.Nodes[i]
			nj := result.Nodes[j]
			if ni.Sequence != nil && nj.Sequence != nil && ni.Sequence == nj.Sequence {
				continue
			}
			if ni.IsDescendantOf(nj) || nj.IsDescendantOf(ni) {
				continue
			}
			if ni.DoesOverlapExact(nj) {
				return fmt.Errorf("nodes %v and %v overlap but are not container and child", ni.ID, nj.ID)
			}

		}
	}
	return nil
}
