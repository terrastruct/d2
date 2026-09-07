package layoutgraph

import (
	"math"
	"strings"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

type Sequence struct {
	Vessel         *Node
	Nodes          []*Node
	Graph          *Graph
	EdgeAbductions []*EdgeAbduction
	Container      *Node
}

// findAbductedNodeByEdge searches for the edge abduction related to the given edge and then returns the original node
func (s *Sequence) findAbductedNodeByEdge(e *Edge) *Node {
	for _, ea := range s.EdgeAbductions {
		if e != ea.Edge {
			continue
		}
		if ea.CurrentFrom == s.Vessel {
			return ea.OriginallyFrom
		}
		if ea.CurrentTo == s.Vessel {
			return ea.OriginallyTo
		}
	}
	return nil
}

// SyncGeometry resizes the sequence vessel and arranges its visible steps.
func (s *Sequence) SyncGeometry() {
	if err := s.SyncGeometryWithWork(unmeteredGroupGeometry); err != nil {
		panic(err)
	}
}

// SyncGeometryWithWork resizes the sequence vessel and arranges its visible
// steps through caller-owned cancellation and movement operations.
func (s *Sequence) SyncGeometryWithWork(work SharedWorkStepper) error {
	if work == nil {
		return invariant.New("sequence geometry requires work accounting")
	}
	if err := work.Step(); err != nil {
		return err
	}
	if s == nil || s.Vessel == nil {
		return invariant.New("sequence is missing its vessel")
	}
	if err := s.resizeWithWork(work); err != nil {
		return err
	}
	return s.arrangeStepsWithWork(work)
}

func (s *Sequence) resizeWithWork(work workStepper) error {
	width := 0.0
	offset := 0.0
	height := 0.0
	for _, n := range s.Nodes {
		if err := work.Step(); err != nil {
			return err
		}
		if n == nil {
			return invariant.New("sequence contains a nil step")
		}
		width = math.Max(width, offset+math.Max(0, n.Width))
		offset += SequenceAdvance(n.Width)
		height = math.Max(height, math.Max(0, n.Height))
	}
	s.Vessel.Width = width
	s.Vessel.Height = height
	return nil
}

// ArrangeSteps places visible sequence steps relative to the vessel.
func (s *Sequence) ArrangeSteps() {
	if err := s.arrangeStepsWithWork(unmeteredGroupGeometry); err != nil {
		panic(err)
	}
}

func (s *Sequence) arrangeStepsWithWork(work workStepper) error {
	// hasn't been initialized yet
	if s.Vessel.TopLeft == nil {
		return work.Finish()
	}

	tl := s.Vessel.TopLeft.Copy()
	for _, n := range s.Nodes {
		if err := work.Step(); err != nil {
			return err
		}
		n.TopLeft = tl.Copy()
		tl.X += SequenceAdvance(n.Width)
	}
	return work.Finish()
}

// PlaceVessel positions the sequence vessel at the top-left of its steps.
func (s *Sequence) PlaceVessel() {
	topLeft := geo.NewPoint(math.Inf(1), math.Inf(1))
	for _, node := range s.Nodes {
		if node.TopLeft == nil {
			return
		}
		topLeft.X = math.Min(topLeft.X, node.TopLeft.X)
		topLeft.Y = math.Min(topLeft.Y, node.TopLeft.Y)
	}
	s.Vessel.TopLeft = topLeft
}

// SequenceAdvance is the horizontal distance from one step's top-left to
// the next. The rendered Step shape clamps its wedge to half of the width when
// the requested width is at most STEP_WEDGE_WIDTH, so fixed-size narrow steps
// must use the same geometry instead of always overlapping by the full wedge.
func SequenceAdvance(width float64) float64 {
	if width <= 0 {
		return 0
	}
	wedge := shape.STEP_WEDGE_WIDTH
	if width <= wedge {
		wedge = width / 2
	}
	return width - wedge
}

// SyncSequences synchronizes every active sequence in nested graph order.
func (g *Graph) SyncSequences() {
	if len(g.Sequences) == 0 {
		return
	}

	sync := func(n *Node) {
		if sequence, is := g.Sequences[n]; is {
			sequence.SyncGeometry()
		}
	}
	for _, n := range g.Nodes {
		n.rdfsWalk(sync)
	}
}

func (s *Sequence) first() *Node {
	return s.Nodes[0]
}
func (s *Sequence) last() *Node {
	return s.Nodes[len(s.Nodes)-1]
}

// First and Last expose the endpoints needed by sequence-aware routing.
func (s *Sequence) First() *Node { return s.first() }
func (s *Sequence) Last() *Node  { return s.last() }

func (s *Sequence) isActive() bool {
	return s != nil && s.Vessel.Graph != nil
}

func (s *Sequence) DebugID() string {
	nodeIDs := []string{}
	for _, n := range s.Nodes {
		nodeIDs = append(nodeIDs, n.DebugID())
	}
	return "[" + strings.Join(nodeIDs, ", ") + "]"
}
