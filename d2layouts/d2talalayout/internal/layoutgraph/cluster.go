package layoutgraph

import (
	"math"
	"strings"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/lib/geo"
)

// Cluster is a group of nodes that
// - have the same size
// - are all adjacent to another node in the cluster
// - are evenly spaced apart from each other
// - are arranged according to a ClusterArrangement
type Cluster struct {
	// Vessel is the node that gets moved around during node placement and encapsulates the cluster nodes
	Vessel             *Node
	Nodes              []*Node
	Arrangement        ClusterArrangement
	DesiredArrangement ClusterArrangement
	Graph              *Graph
	EdgeAbductions     []*EdgeAbduction
	Padding            float64
	FixedSize          bool

	Container *Node
}

type ClusterArrangement string

func (ca ClusterArrangement) Flip() ClusterArrangement {
	if ca == Row {
		return Column
	}
	return Row
}

const (
	Row    ClusterArrangement = "Row"
	Column ClusterArrangement = "Column"
)

// Resize makes the cluster nodes consistent in size, and sizes the vessel to exact fit of cluster nodes
// It should be called after any potential resizing, which may happen if a child node is a container and gets resized
func (c *Cluster) Resize(vessel *Node) {
	if err := c.resizeWithWork(vessel, unmeteredGroupGeometry); err != nil {
		panic(err)
	}
}

func (c *Cluster) resizeWithWork(vessel *Node, work workStepper) error {
	if c == nil || vessel == nil {
		return invariant.New("cluster is missing its vessel")
	}
	if !c.FixedSize {
		nodeWidth, nodeHeight, err := c.maximumsWithWork(work)
		if err != nil {
			return err
		}

		for _, node := range c.Nodes {
			if err := work.Step(); err != nil {
				return err
			}
			node.Width = nodeWidth
			node.Height = nodeHeight
		}
	}
	nodeWidth, nodeHeight, err := c.maximumsWithWork(work)
	if err != nil {
		return err
	}
	if c.Arrangement == Row {
		vessel.Width = nodeWidth*float64(len(c.Nodes)) + c.Padding*float64(len(c.Nodes)-1)
		vessel.Height = nodeHeight
	}

	if c.Arrangement == Column {
		vessel.Width = nodeWidth
		vessel.Height = nodeHeight*float64(len(c.Nodes)) + c.Padding*float64(len(c.Nodes)-1)
	}
	return nil
}

func (c *Cluster) maximumsWithWork(work workStepper) (float64, float64, error) {
	maxWidth, maxHeight := 0.0, 0.0
	for _, node := range c.Nodes {
		if err := work.Step(); err != nil {
			return 0, 0, err
		}
		if node == nil {
			return 0, 0, invariant.New("cluster contains a nil node")
		}
		maxWidth = math.Max(maxWidth, node.Width)
	}
	for _, node := range c.Nodes {
		if err := work.Step(); err != nil {
			return 0, 0, err
		}
		maxHeight = math.Max(maxHeight, node.Height)
	}
	return maxWidth, maxHeight, work.Finish()
}

// ArrangeClusterNodes moves the inner cluster nodes to their respective positions within the cluster
// This should be called after vessels move
// This is idempotent -- calling it after it's in the right position results in 0's for movements
func (c *Cluster) ArrangeClusterNodes() {
	if err := c.arrangeNodesWithWork(unmeteredGroupGeometry); err != nil {
		panic(err)
	}
}

func (c *Cluster) arrangeNodesWithWork(work ClusterGeometryWork) error {
	// Hasn't been initialized yet, which is fine since we arrange all cluster nodes every time for simplicity
	if c.Vessel.TopLeft == nil {
		return work.Finish()
	}

	if c.Arrangement == Row {
		position := c.Vessel.TopLeft.X
		vesselCenter := c.Vessel.TopLeft.Y + c.Vessel.Height/2

		for _, node := range c.Nodes {
			if err := work.Step(); err != nil {
				return err
			}
			if node.TopLeft != nil {
				dx := position - node.TopLeft.X
				dy := math.Round(vesselCenter - (node.TopLeft.Y + node.Height/2))
				if err := work.MoveNodeWithChildren(node, dx, dy); err != nil {
					return err
				}
			} else {
				node.TopLeft = geo.NewPoint(position, math.Round(vesselCenter-node.Height/2))
				if err := work.PositionContainerChildren(node); err != nil {
					return err
				}
			}
			position += node.Width + c.Padding
		}
	}

	if c.Arrangement == Column {
		position := c.Vessel.TopLeft.Y
		vesselCenter := c.Vessel.TopLeft.X + c.Vessel.Width/2

		for _, node := range c.Nodes {
			if err := work.Step(); err != nil {
				return err
			}
			if node.TopLeft != nil {
				dx := math.Round(vesselCenter - (node.TopLeft.X + node.Width/2))
				dy := position - node.TopLeft.Y
				if err := work.MoveNodeWithChildren(node, dx, dy); err != nil {
					return err
				}
			} else {
				node.TopLeft = geo.NewPoint(math.Round(vesselCenter-node.Width/2), position)
				if err := work.PositionContainerChildren(node); err != nil {
					return err
				}
			}
			position += node.Height + c.Padding
		}
	}
	return work.Finish()
}

func (cluster *Cluster) DebugID() string {
	nodeIDs := []string{}
	for _, n := range cluster.Nodes {
		nodeIDs = append(nodeIDs, n.DebugID())
	}
	return "[" + strings.Join(nodeIDs, ", ") + "]; Arrangement: " + string(cluster.Arrangement)
}

// SyncGeometry resizes the cluster vessel and arranges its visible members.
func (c *Cluster) SyncGeometry() {
	if err := c.SyncGeometryWithWork(unmeteredGroupGeometry); err != nil {
		panic(err)
	}
}

// SyncGeometryWithWork resizes the cluster vessel and arranges its visible
// members through caller-owned cancellation and movement operations.
func (c *Cluster) SyncGeometryWithWork(work ClusterGeometryWork) error {
	if work == nil {
		return invariant.New("cluster geometry requires work accounting")
	}
	if err := work.Step(); err != nil {
		return err
	}
	if c == nil || c.Vessel == nil {
		return invariant.New("cluster is missing its vessel")
	}
	if err := c.resizeWithWork(c.Vessel, work); err != nil {
		return err
	}
	return c.arrangeNodesWithWork(work)
}

// SyncClusters synchronizes every active cluster in nested graph order.
func (g *Graph) SyncClusters() {
	if len(g.Clusters) == 0 {
		return
	}

	sync := func(n *Node) {
		if n.isClusterVessel {
			g.Clusters[n].SyncGeometry()
		}
	}
	for _, n := range g.Nodes {
		n.rdfsWalk(sync)
	}
}

// isActive is true while a Cluster's vessel is in the graph replacing the cluster nodes.
// The vessel is a convenience during node placement, which is not kept in sync after deactivation.
func (c *Cluster) isActive() bool {
	return c != nil && c.Vessel.Graph != nil
}
