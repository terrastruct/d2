package layoutgraph

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/nodeshape"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

const (
	defaultIconSize = 32.0
	threeDOffset    = 15.0
	multipleOffset  = 10.0
)

// HasLeakyEdge checks if
// 1. node is a container
// 2. a descendant has an edge going outside the container
func (node *Node) HasLeakyEdge() bool {
	if !node.isContainer {
		return false
	}

	hasLeakyEdge := false
	node.dfsWalk(func(descendant *Node) (stop bool) {
		if descendant == node {
			return false
		}
		for _, e := range descendant.Edges {
			adj := descendant.adjacent(e)
			if !adj.isDescendantOf(node) {
				hasLeakyEdge = true
				return true
			}
		}
		return false
	})
	return hasLeakyEdge
}

func (node *Node) fitNodeToGraph(g *Graph, padding Spacing) {
	tl, br := Nodes(g.Nodes).fixedBounds()
	node.expandForLabels(tl, br)
	node.fitToBoundingBox(tl, br, padding)
}

func (node *Node) fitToBoundingBox(tl, br *geo.Point, padding Spacing) {
	width := br.X - tl.X
	height := br.Y - tl.Y
	if node.Label != nil && !node.Label.Position.IsOutside() {
		minWidth := node.Label.Width - padding.left - padding.right + label.PADDING*4
		minHeight := node.Label.Height - padding.top - padding.bottom + label.PADDING*4
		if node.DesiredWidth == nil {
			width = math.Max(width, minWidth)
		}
		if node.DesiredHeight == nil {
			height = math.Max(height, minHeight)
		}
	}
	// TODO move type Spacing to d2 and update GetDimensionsToFit for all shapes
	fitWidth, fitHeight := node.GetDimensionsToFit(width, height, padding.left+padding.right, padding.top+padding.bottom)
	if node.DesiredWidth != nil {
		node.Width = math.Max(fitWidth, *node.DesiredWidth)
	} else {
		node.Width = fitWidth
	}
	if node.DesiredHeight != nil {
		node.Height = math.Max(fitHeight, *node.DesiredHeight)
	} else {
		node.Height = fitHeight
	}
}

func (node *Node) bounds(allNodes Nodes) (*geo.Point, *geo.Point) {
	return node.boundsWithRounding(allNodes, true)
}

func (node *Node) boundsWithRounding(allNodes Nodes, roundDimensions bool) (*geo.Point, *geo.Point) {
	tl, br := node.boundingBoxValues(allNodes, roundDimensions)
	return &tl, &br
}

func (node *Node) boundingBoxValues(allNodes Nodes, roundDimensions bool) (geo.Point, geo.Point) {
	tl := *node.TopLeft
	br := geo.Point{X: tl.X + node.Width, Y: tl.Y + node.Height}
	if roundDimensions {
		br.X = math.Round(br.X)
		br.Y = math.Round(br.Y)
	}
	if dx, dy := node.modifierElementAdjustments(); dx != 0 || dy != 0 {
		tl.Y -= dy
		br.X += dx
	}
	tl.X -= node.LoopOffsets[geo.Left]
	tl.Y -= node.LoopOffsets[geo.Top]
	br.X += node.LoopOffsets[geo.Right]
	br.Y += node.LoopOffsets[geo.Bottom]

	if node.Label != nil && node.Label.Position.IsOutside() && allNodes != nil {
		labelTL := node.LabelTopLeft(node.Label.Position, node.Label.Width, node.Label.Height)

		// include 2x padding so that outside labels will always be closest to their own node
		// . ┌─────┐                ┌─────┐
		// . │     │ ┌───────────┐  │     │
		// . │  A  │ │ A's label │  │  B  │
		// . │     │ └───────────┘  │     │
		// . └─────┘                └─────┘
		// .       ├─┤           ├──┤
		// .    label.PADDING   2xlabel.PADDING
		boundaryOutsidePadding := float64(label.PADDING)
		// If the node is at the boundary, there won't be ambiguity
		outsidePadding := 2. * label.PADDING

		if labelTL.X < tl.X {
			if allNodes.Leftmost(node) {
				tl.X = math.Floor(labelTL.X - boundaryOutsidePadding)
			} else {
				tl.X = math.Floor(labelTL.X - outsidePadding)
			}
		}
		if labelTL.Y < tl.Y {
			if allNodes.Topmost(node) {
				tl.Y = math.Floor(labelTL.Y - boundaryOutsidePadding)
			} else {
				tl.Y = math.Floor(labelTL.Y - outsidePadding)
			}
		}
		if labelTL.X > br.X {
			if allNodes.Rightmost(node) {
				br.X = math.Ceil(labelTL.X + node.Label.Width + boundaryOutsidePadding)
			} else {
				br.X = math.Ceil(labelTL.X + node.Label.Width + outsidePadding)
			}
		}

		if labelTL.Y > br.Y {
			if allNodes.Bottommost(node) {
				br.Y = math.Ceil(labelTL.Y + node.Label.Height + boundaryOutsidePadding)
			} else {
				br.Y = math.Ceil(labelTL.Y + node.Label.Height + outsidePadding)
			}
		}
	}
	if node.Icon != nil && node.shapeType != imageType && node.Icon.Position.IsOutside() && allNodes != nil {
		iconSize := float64(MaxIconSize)
		iconTL := node.LabelTopLeft(node.Icon.Position, iconSize, iconSize)

		outsidePadding := 2. * label.PADDING

		left := math.Floor(iconTL.X - outsidePadding)
		if left < tl.X {
			tl.X = left
		}
		top := math.Floor(iconTL.Y - outsidePadding)
		if top < tl.Y {
			tl.Y = top
		}
		right := math.Ceil(iconTL.X + iconSize + outsidePadding)
		if right > br.X {
			br.X = right
		}
		bottom := math.Ceil(iconTL.Y + iconSize + outsidePadding)
		if bottom > br.Y {
			br.Y = bottom
		}
	}
	return tl, br
}

// TODO: verify whether this correctly finds all overlapping ports.
func (n1 *Node) overlappingPorts(n2 *Node) map[geo.Point]bool {
	overlaps := make(map[geo.Point]bool)

	// Don't count container overlaps
	if n1.isDescendantOf(n2) || n2.isDescendantOf(n1) {
		return overlaps
	}

	n1Ports := n1.ports()
	n2Ports := n2.ports()
	for _, n1Port := range n1Ports {
		found := slices.Contains(n2Ports, n1Port)
		if found {
			overlaps[n1Port] = true
		}
	}

	return overlaps
}

type Nodes []*Node

func (ns Nodes) Leftmost(node *Node) bool {
	for _, n := range ns {
		if n == node {
			continue
		}
		if n.TopLeft == nil {
			continue
		}
		if n.TopLeft.X < node.TopLeft.X {
			return false
		}
	}
	return true
}

func (ns Nodes) Topmost(node *Node) bool {
	for _, n := range ns {
		if n == node {
			continue
		}
		if n.TopLeft == nil {
			continue
		}
		if n.TopLeft.Y < node.TopLeft.Y {
			return false
		}
	}
	return true
}

func (ns Nodes) Rightmost(node *Node) bool {
	for _, n := range ns {
		if n == node {
			continue
		}
		if n.TopLeft == nil {
			continue
		}
		if n.TopLeft.X+n.Width > node.TopLeft.X+node.Width {
			return false
		}
	}
	return true
}

func (ns Nodes) Bottommost(node *Node) bool {
	for _, n := range ns {
		if n == node {
			continue
		}
		if n.TopLeft == nil {
			continue
		}
		if n.TopLeft.Y+n.Height > node.TopLeft.Y+node.Height {
			return false
		}
	}
	return true
}

func (ns Nodes) DebugID() string {
	nodeIDs := []string{}
	for _, n := range ns {
		nodeIDs = append(nodeIDs, n.DebugID())
	}
	return "[" + strings.Join(nodeIDs, ", ") + "]"
}

func (ns Nodes) setGraphReference(g *Graph) {
	for _, n := range ns {
		n.Graph = g
		for _, child := range g.allDescendantNodes(n, true) {
			child.Graph = g
		}
	}
}

func (nodes Nodes) adjacentCount() int {
	sum := 0
	for _, n := range nodes {
		sum += len(n.Edges)
	}
	return sum
}

// 1. Algorithm is to seed clusters with nodes which connected/near and further than a threshold distance
// 2. Do recursive scans in the threshold distance perimeter
// ---- if the scan finds a node which is not part of a cluster, it becomes part of the scanning node's cluster
// ---- if the scan finds a node which is part of a cluster, that cluster is merged into the scanning node's cluster
func (nodes Nodes) clusters(distanceThreshold float64) [][]*Node {
	clusters, err := nodes.clustersWithWorkGuard(distanceThreshold, nil)
	if err != nil {
		panic(err)
	}
	return clusters
}

func (nodes Nodes) clustersWithWorkGuard(distanceThreshold float64, guard *limits.WorkGuard) ([][]*Node, error) {
	charge := func(units int64) error {
		if guard == nil {
			return nil
		}
		return guard.Add(units)
	}
	finish := func() error {
		if guard == nil {
			return nil
		}
		return guard.Finish()
	}
	chargeSort := func(length int) error {
		for width := 1; width < length; width *= 2 {
			if err := charge(int64(length)); err != nil {
				return err
			}
			if width > length/2 {
				break
			}
		}
		return nil
	}

	nodeToCluster := make(map[*Node]int)

	nextGeneratedClusterID := 0
	createCluster := func(node *Node) {
		if _, in := nodeToCluster[node]; !in {
			clusterID := nextGeneratedClusterID
			nextGeneratedClusterID++
			nodeToCluster[node] = clusterID
		}
	}

	seedNodes := make([]*Node, 0)
	maybeAddSeedNodes := func(n, adj *Node) {
		d := n.distance(adj, true)
		if geo.PrecisionCompare(d, distanceThreshold, geo.PRECISION) > 0 {
			seedNodes = append(seedNodes, n, adj)
		}
	}
	// Step 1
	for _, n := range nodes {
		if err := charge(1 + int64(len(n.Edges)) + int64(len(n.Nears))); err != nil {
			return nil, err
		}
		for _, e := range n.Edges {
			adj := n.adjacent(e)
			maybeAddSeedNodes(n, adj)
		}
		if err := chargeSort(len(n.Nears)); err != nil {
			return nil, err
		}
		for _, near := range n.orderedNears() {
			if near.Cluster != nil {
				near = near.Cluster.Vessel
			} else if near.Sequence != nil {
				near = near.Sequence.Vessel
			} else if tree, is := n.Graph.NodeToTree[near]; is {
				if guard == nil {
					near = tree.root().sentinelNode()
				} else {
					for tree.Parent != nil {
						if err := guard.Step(); err != nil {
							return nil, err
						}
						tree = tree.Parent
					}
					near = tree.sentinelNode()
				}
			}
			if near.Container != n.Container {
				continue
			}
			maybeAddSeedNodes(n, near)
		}
	}

	if len(seedNodes) < 2 {
		return nil, finish()
	}

	// Step 2
	var mergeNodesInPerimeter func(n *Node) error
	mergeNodesInPerimeter = func(n *Node) error {
		if err := charge(int64(len(nodes))); err != nil {
			return err
		}
		clusterID := nodeToCluster[n]
		for _, otherN := range nodes {
			if otherN == n {
				continue
			}
			if otherN.FixedTopLeft != nil {
				continue
			}
			// TODO: consider caching this distance & comparison as it's the same computed a few lines above
			d := n.distance(otherN, true)
			if geo.PrecisionCompare(d, distanceThreshold, geo.PRECISION) > 0 {
				continue
			}
			otherClusterID, in := nodeToCluster[otherN]
			if in && otherClusterID == clusterID {
				continue
			}

			nodeToCluster[otherN] = nodeToCluster[n]
			if !in {
				if err := mergeNodesInPerimeter(otherN); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := charge(int64(len(seedNodes))); err != nil {
		return nil, err
	}
	for _, node := range seedNodes {
		if node.FixedTopLeft != nil {
			continue
		}
		createCluster(node)
		if err := mergeNodesInPerimeter(node); err != nil {
			return nil, err
		}
	}

	if err := charge(int64(len(nodes))); err != nil {
		return nil, err
	}
	if !nodes.hasFixedNode() {
		if len(nodeToCluster) != len(nodes) {
			panic(fmt.Sprintf("Clustered nodes: %v. Total: %v", len(nodeToCluster), len(nodes)))
		}
	}

	clusterMapping := make(map[int][]*Node)
	if err := charge(int64(len(nodeToCluster))); err != nil {
		return nil, err
	}
	for node, clusterID := range nodeToCluster {
		_, in := clusterMapping[clusterID]
		if !in {
			clusterMapping[clusterID] = make([]*Node, 0)
		}
		clusterMapping[clusterID] = append(clusterMapping[clusterID], node)
	}

	for _, cluster := range clusterMapping {
		if err := chargeSort(len(cluster)); err != nil {
			return nil, err
		}
		sortNodesByID(cluster)
	}

	clusterIDs := make([]int, 0, len(clusterMapping))
	if err := charge(int64(len(clusterMapping))); err != nil {
		return nil, err
	}
	for clusterID := range clusterMapping {
		clusterIDs = append(clusterIDs, clusterID)
	}
	if err := chargeSort(len(clusterIDs)); err != nil {
		return nil, err
	}
	sort.Ints(clusterIDs)

	clusters := make([][]*Node, 0)
	if err := charge(int64(len(clusterIDs))); err != nil {
		return nil, err
	}
	for _, clusterID := range clusterIDs {
		clusters = append(clusters, clusterMapping[clusterID])
	}

	if err := finish(); err != nil {
		return nil, err
	}
	return clusters, nil
}

func (nodes Nodes) center() *geo.Point {
	tl, br := nodes.bounds()
	return geo.NewPoint(tl.X+(br.X-tl.X)/2, tl.Y+(br.Y-tl.Y)/2)
}

func (nodes Nodes) centerWithWorkGuard(guard *limits.WorkGuard) (*geo.Point, error) {
	tl, br, err := nodes.boundsWithWorkGuard(guard)
	if err != nil {
		return nil, err
	}
	return geo.NewPoint(tl.X+(br.X-tl.X)/2, tl.Y+(br.Y-tl.Y)/2), nil
}

func (nodes Nodes) bounds() (*geo.Point, *geo.Point) {
	return nodes.boundingBox(true)
}

func (nodes Nodes) boundsWithWorkGuard(guard *limits.WorkGuard) (*geo.Point, *geo.Point, error) {
	if len(nodes) == 0 {
		if err := guard.Finish(); err != nil {
			return nil, nil, err
		}
		return geo.NewPoint(math.Inf(-1), math.Inf(-1)), geo.NewPoint(math.Inf(1), math.Inf(1)), nil
	}
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)

	for _, node := range nodes {
		if err := guard.Step(); err != nil {
			return nil, nil, err
		}
		if node.TopLeft == nil {
			return nil, nil, nil
		}
		if node.Label != nil && node.Label.Position.IsOutside() {
			// An outside label can scan the complete peer set along each axis.
			if err := guard.Add(2 * int64(len(nodes))); err != nil {
				return nil, nil, err
			}
		}
		tl, br := node.boundingBoxValues(nodes, true)
		minX = math.Min(minX, tl.X)
		minY = math.Min(minY, tl.Y)
		maxX = math.Max(maxX, br.X)
		maxY = math.Max(maxY, br.Y)
	}

	if err := guard.Finish(); err != nil {
		return nil, nil, err
	}
	return geo.NewPoint(minX, minY), geo.NewPoint(maxX, maxY), nil
}

func (nodes Nodes) unroundedBounds() (*geo.Point, *geo.Point) {
	return nodes.boundingBox(false)
}

func (nodes Nodes) boundingBox(roundDimensions bool) (*geo.Point, *geo.Point) {
	if len(nodes) == 0 {
		return geo.NewPoint(math.Inf(-1), math.Inf(-1)), geo.NewPoint(math.Inf(1), math.Inf(1))
	}
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)

	for _, node := range nodes {
		if node.TopLeft == nil {
			return nil, nil
		}
		tl, br := node.boundingBoxValues(nodes, roundDimensions)
		minX = math.Min(minX, tl.X)
		minY = math.Min(minY, tl.Y)
		maxX = math.Max(maxX, br.X)
		maxY = math.Max(maxY, br.Y)
	}

	return geo.NewPoint(minX, minY), geo.NewPoint(maxX, maxY)
}

// for nodes in the same container, return the bounding box accounting for any fixed node offset
func (nodes Nodes) fixedBounds() (*geo.Point, *geo.Point) {
	tl, br := nodes.bounds()
	if fixedOrigin := nodes.fixedOrigin(); fixedOrigin != nil {
		tl = fixedOrigin
	}
	return tl, br
}

func (nodes Nodes) unroundedFixedBounds() (*geo.Point, *geo.Point) {
	tl, br := nodes.unroundedBounds()
	if fixedOrigin := nodes.fixedOrigin(); fixedOrigin != nil {
		tl = fixedOrigin
	}
	return tl, br
}

func (ns Nodes) intersectsNode(edge *Edge) bool {
	// Check for intersections on other nodes
	for _, otherNode := range ns {
		// Can intersect with a container if one endpoint is inside it
		if edge.From.isDescendantOf(otherNode) {
			continue
		}
		if edge.To.isDescendantOf(otherNode) {
			continue
		}

		for i := 0; i < len(edge.Points)-1; i++ {
			if otherNode.passesThrough(edge.Points[i], edge.Points[i+1]) {
				return true
			}
		}
	}
	return false
}

type Spacing struct {
	top, bottom, left, right float64
}

// Node positions and dimensions for shapes should always be whole numbers
// When used in OVGs for edge routing, they can take decimal values
type Node struct {
	D2ID *string
	ID   EntityID
	geo.Box
	Edges []*Edge
	// Nears is a list of nodes which this node wants to position itself close to one of
	// They're different from edges that have additional heuristics like alignment, symmetry, tree/cluster-formation, etc
	// And we don't reward being closer to all of them
	Nears    map[*Node]struct{}
	Graph    *Graph
	FontSize *int

	// This defines the snap points
	nodeshape.Shape
	shapeType nodeshape.Kind

	ForceHierarchy bool

	Container      *Node
	Cluster        *Cluster
	Sequence       *Sequence
	HerdAssignment *HerdAssignment
	Hierarchy      *Hierarchy

	Label *Label
	Icon  *Icon

	FixedTopLeft *geo.Point

	// The node should be be these dimensions, but containers can still grow to fit their children
	DesiredWidth  *float64
	DesiredHeight *float64

	// LoopOffsets contains the distance (offset) from the node bounding box
	// to loop borders as shown in the example below.
	// For diagonals, take the max of the coordinates: TopLeft=max(Top, Left)
	//         left
	//        |----|                right
	//      ┬ ┌─────────┐        |--------|
	//  top | │ ┌───────┤     ┌────┐
	//      ┴ │ │  ┌────▼─────┴──┐ a label
	//        │ │  │             ◄─┘
	//        └─┴──┤     node    │
	//             │             │
	//          ┌──►             │
	//          │  │             │
	// bottom ┬ │  └──┬──────────┘
	//        ┴ └─────┘
	LoopOffsets map[geo.Orientation]float64

	margin  Spacing
	padding Spacing

	isClusterVessel bool
	isContainer     bool

	// to adjust where connections are for 3d/multiple styles
	Is3D       bool
	IsMultiple bool

	// Connections can go through invisible nodes.
	// For example, opacity 0 grid cells used for spacing
	IsInvisible bool

	// LongDistanceNeighborRequirements records optimizer-derived parallel-edge
	// spacing by adjacent node.
	LongDistanceNeighborRequirements map[*Node]LongDistanceNeighborRequirements
}

func (n *Node) DebugID() string {
	if n == nil {
		return "nil"
	}
	if n.D2ID != nil {
		return *n.D2ID
	}
	if n.isClusterVessel {
		return "Cluster vessel of: " + n.Graph.Clusters[n].DebugID()
	}
	if n.Graph != nil && len(n.Graph.Sequences) > 0 {
		if s, ok := n.Graph.Sequences[n]; ok {
			return "Sequence vessel of: " + s.DebugID()
		}
	}
	return strconv.FormatInt(n.ID, 10)
}

func (n *Node) Level() int {
	if n == nil {
		return 0
	}
	if n.Cluster.isActive() {
		return n.Cluster.Vessel.Level()
	}
	if n.Sequence.isActive() {
		return n.Sequence.Vessel.Level()
	}
	return 1 + n.Container.Level()
}

func (n *Node) adjacent(e *Edge) *Node {
	if n == e.From {
		return e.To
	}
	return e.From
}

func NewNode(id EntityID, width, height float64) *Node {
	node := &Node{
		ID: id,
		Box: geo.Box{
			Width:  width,
			Height: height,
		},
	}
	node.Edges = make([]*Edge, 0)
	node.Nears = make(map[*Node]struct{})
	node.SetShape("")
	return node
}

func (n *Node) InitIcon() {
	n.Icon = &Icon{}
}

func (n *Node) SetShape(shapeType string) {
	// The shape retains this exact box pointer so its geometry follows node
	// moves and resizes.
	s, kind, ok := nodeshape.New(shapeType, &n.Box)
	if ok {
		n.Shape = s
		n.shapeType = kind
	}
}

// ShapeType reports the node's canonical shape type.
func (n *Node) ShapeType() string {
	return n.Shape.GetType()
}

// NumColumns reports the number of columns in a table-shaped node.
func (n *Node) NumColumns() int {
	return nodeshape.NumColumns(n.Shape)
}

func (n *Node) SetNumColumns(numColumns int) {
	nodeshape.SetNumColumns(n.Shape, numColumns)
}

func (n *Node) AddNear(otherN *Node) {
	if n == nil || otherN == nil || n == otherN {
		return
	}
	if n.Nears == nil {
		n.Nears = make(map[*Node]struct{})
	}
	if otherN.Nears == nil {
		otherN.Nears = make(map[*Node]struct{})
	}
	n.Nears[otherN] = struct{}{}
	otherN.Nears[n] = struct{}{}
}

func (node *Node) entityID() EntityID {
	if node == nil {
		return 0
	}
	return node.ID
}

func (n1 *Node) deltaTo(n2 *Node, atPoint *geo.Point) int {
	delta, err := n1.deltaToGuarded(n2, atPoint, nil)
	if err != nil {
		panic(err)
	}
	return delta
}

func (n1 *Node) deltaToGuarded(n2 *Node, atPoint *geo.Point, guard workStepper) (int, error) {
	if n1 == nil || n2 == nil || atPoint == nil {
		return 0, invariant.New("spacing check received incomplete nodes")
	}
	// n1 may have multiple edges to n2. We want to get the largest distance that we need to maintain
	maxEdgeWidth := math.MinInt
	maxEdgeHeight := math.MinInt
	isConnected := false
	for _, e := range n1.Edges {
		if guard != nil {
			if err := guard.Step(); err != nil {
				return 0, err
			}
		}
		if e == nil || e.From == nil || e.To == nil {
			return 0, invariant.New("spacing check encountered an incomplete edge")
		}
		// TODO (Sun Apr 10 16:07:25 2022) this doesn't take edge abductions into consideration, so it may not find a minWidth to a child
		if n1.adjacent(e) == n2 {
			isConnected = true
			if e.MinWidth > maxEdgeWidth {
				maxEdgeWidth = e.MinWidth
			}
			if e.MinHeight > maxEdgeHeight {
				maxEdgeHeight = e.MinHeight
			}
		}
	}

	var horizontalDelta, verticalDelta int
	if isConnected {
		horizontalDelta = ConnectedNodeGap
		verticalDelta = ConnectedNodeGap
	} else {
		horizontalDelta = NodeGap
		verticalDelta = NodeGap
	}

	if n1.shapeType == tableType || n2.shapeType == tableType {
		horizontalDelta = TableNodeGap
	}

	if maxEdgeHeight > verticalDelta {
		verticalDelta = maxEdgeHeight
	}
	if maxEdgeWidth > horizontalDelta {
		horizontalDelta = maxEdgeWidth
	}

	// Ordinary unlabeled boxes often require the same gap in every direction.
	// In that case the orientation and directional-margin switches cannot
	// affect the result; retain the edge scan above for its validation and work.
	if horizontalDelta == verticalDelta && len(n1.LoopOffsets) == 0 && len(n2.LoopOffsets) == 0 &&
		n1.margin == (Spacing{}) && n2.margin == (Spacing{}) {
		return horizontalDelta, nil
	}
	o := n1.orientationAtPoint(n2, atPoint)

	if len(n1.LoopOffsets) > 0 || len(n2.LoopOffsets) > 0 {
		loopDeltas := NodeGap
		if len(n1.LoopOffsets) > 0 {
			loopDeltas += int(n1.LoopOffsets[o.GetOpposite()])
		}
		if len(n2.LoopOffsets) > 0 {
			loopDeltas += int(n2.LoopOffsets[o])
		}
		if loopDeltas > horizontalDelta {
			horizontalDelta = loopDeltas
		}
		if loopDeltas > verticalDelta {
			verticalDelta = loopDeltas
		}
	}

	var n1LabelWidth, n1LabelHeight, n2LabelWidth, n2LabelHeight int
	switch o.GetOpposite() {
	case geo.Bottom:
		n1LabelHeight = int(n1.margin.bottom)
	case geo.Top:
		n1LabelHeight = int(n1.margin.top)
	case geo.Right:
		n1LabelWidth = int(n1.margin.right)
	case geo.Left:
		n1LabelWidth = int(n1.margin.left)
	case geo.BottomLeft:
		n1LabelWidth = int(n1.margin.left)
		n1LabelHeight = int(n1.margin.bottom)
	case geo.BottomRight:
		n1LabelWidth = int(n1.margin.right)
		n1LabelHeight = int(n1.margin.bottom)
	case geo.TopLeft:
		n1LabelWidth = int(n1.margin.left)
		n1LabelHeight = int(n1.margin.top)
	case geo.TopRight:
		n1LabelWidth = int(n1.margin.right)
		n1LabelHeight = int(n1.margin.top)
	}
	switch o {
	case geo.Bottom:
		n2LabelHeight = int(n2.margin.bottom)
	case geo.Top:
		n2LabelHeight = int(n2.margin.top)
	case geo.Right:
		n2LabelWidth = int(n2.margin.right)
	case geo.Left:
		n2LabelWidth = int(n2.margin.left)
	case geo.BottomLeft:
		n2LabelWidth = int(n2.margin.left)
		n2LabelHeight = int(n2.margin.bottom)
	case geo.BottomRight:
		n2LabelWidth = int(n2.margin.right)
		n2LabelHeight = int(n2.margin.bottom)
	case geo.TopLeft:
		n2LabelWidth = int(n2.margin.left)
		n2LabelHeight = int(n2.margin.top)
	case geo.TopRight:
		n2LabelWidth = int(n2.margin.right)
		n2LabelHeight = int(n2.margin.top)
	}
	if horizontalDelta < n1LabelWidth+n2LabelWidth {
		horizontalDelta = n1LabelWidth + n2LabelWidth
	}
	if verticalDelta < n1LabelHeight+n2LabelHeight {
		verticalDelta = n1LabelHeight + n2LabelHeight
	}

	switch o {
	case geo.Top, geo.Bottom:
		return verticalDelta, nil
	case geo.Left, geo.Right:
		return horizontalDelta, nil
	default:
		if horizontalDelta < verticalDelta {
			return horizontalDelta, nil
		}
		return verticalDelta, nil
	}
}

func (n *Node) UpdateSpacing() {
	if n.Label != nil && n.Label.PositionFixed() {
		width := n.Label.Width + 2*label.PADDING
		height := n.Label.Height + 2*label.PADDING
		switch n.Label.Position {
		case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
			n.margin.top = height
		case label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
			n.margin.bottom = height
		case label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom:
			n.margin.left = width
		case label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom:
			n.margin.right = width
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			n.padding.top = height
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			n.padding.bottom = height
		case label.InsideMiddleLeft:
			n.padding.left = width
		case label.InsideMiddleRight:
			n.padding.right = width
		}
	}

	if n.Icon != nil && n.Icon.PositionFixed() {
		iconSize := float64(MaxIconSize + 2*label.PADDING)
		switch n.Icon.Position {
		case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight:
			n.margin.top = math.Max(n.margin.top, iconSize)
		case label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
			n.margin.bottom = math.Max(n.margin.bottom, iconSize)
		case label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom:
			n.margin.left = math.Max(n.margin.left, iconSize)
		case label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom:
			n.margin.right = math.Max(n.margin.right, iconSize)
		case label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight:
			n.padding.top = math.Max(n.padding.top, iconSize)
		case label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight:
			n.padding.bottom = math.Max(n.padding.bottom, iconSize)
		case label.InsideMiddleLeft:
			n.padding.left = math.Max(n.padding.left, iconSize)
		case label.InsideMiddleRight:
			n.padding.right = math.Max(n.padding.right, iconSize)
		}
	}

	if dx, dy := n.modifierElementAdjustments(); dx != 0 || dy != 0 {
		n.margin.top += dy
		n.margin.right += dx
	}
}

// . ┌────────────────┐
// . │        ┌───┐   │
// . │      A │ B │   │
// . │        └───┘   │
// . │  ┌─────────────┼──┐
// . └──┼───────C─────┘  │
// .    └────────────────┘
// A covers B but not C, B does not cover A
func (n1 *Node) Covers(n2 *Node) bool {
	return covers(&n1.Box, &n2.Box)
}

// TODO move to d2 geo.Box
func covers(b1, b2 *geo.Box) bool {
	return (b2.TopLeft.X >= b1.TopLeft.X) &&
		(b2.TopLeft.Y >= b1.TopLeft.Y) &&
		(b2.TopLeft.X+b2.Width <= b1.TopLeft.X+b1.Width) &&
		(b2.TopLeft.Y+b2.Height <= b1.TopLeft.Y+b1.Height)
}

func (n1 *Node) doesOverlapCalc(n2 *Node, delta float64) bool {
	return boxesOverlapWithPadding(n1.Box, n2.Box, delta)
}

// boxesOverlapWithPadding reports whether the interiors of two axis-aligned
// boxes overlap, treating a gap smaller than padding as overlap. Boundary
// contact, including a gap exactly equal to padding, is not an overlap.
func boxesOverlapWithPadding(b1, b2 geo.Box, padding float64) bool {
	b1Right := b1.TopLeft.X + b1.Width
	b2Right := b2.TopLeft.X + b2.Width
	if b1.TopLeft.X >= b2Right+padding || b2.TopLeft.X >= b1Right+padding {
		return false
	}

	b1Bottom := b1.TopLeft.Y + b1.Height
	b2Bottom := b2.TopLeft.Y + b2.Height
	return b1.TopLeft.Y < b2Bottom+padding && b2.TopLeft.Y < b1Bottom+padding
}

// DoesOverlapExact reports whether node boxes overlap without clearance padding.
func (n1 *Node) DoesOverlapExact(n2 *Node) bool {
	return n1.doesOverlapCalc(n2, 0)
}

// doesOverlapAt returns whether placing n1 at p overlaps with n2
func (n1 *Node) doesOverlapAt(n2 *Node, p *geo.Point) bool {
	delta := float64(n1.deltaTo(n2, p))
	b1 := geo.Box{TopLeft: p, Width: n1.Width, Height: n1.Height}
	return boxesOverlapWithPadding(b1, n2.Box, delta)
}

func (n1 *Node) doesOverlap(n2 *Node) bool {
	delta := float64(n1.deltaTo(n2, n1.TopLeft))
	return n1.doesOverlapCalc(n2, delta)
}

// for 2 overlapping nodes, return the area of the overlap
func (node *Node) overlapArea(otherNode *Node) float64 {
	// area of overlap is area of rect from middle 2 xs and middle 2 ys
	var xs, ys []float64

	xs = append(xs, node.TopLeft.X)
	xs = append(xs, node.TopLeft.X+node.Width)
	ys = append(ys, node.TopLeft.Y)
	ys = append(ys, node.TopLeft.Y+node.Height)

	xs = append(xs, otherNode.TopLeft.X)
	xs = append(xs, otherNode.TopLeft.X+otherNode.Width)
	ys = append(ys, otherNode.TopLeft.Y)
	ys = append(ys, otherNode.TopLeft.Y+otherNode.Height)

	slices.Sort(xs)
	slices.Sort(ys)

	width := xs[2] - xs[1]
	height := ys[2] - ys[1]

	return width * height
}

func sortNodesByID(nodes []*Node) {
	if len(nodes) < 2 {
		return
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].entityID() < nodes[j].entityID()
	})
}

func (node *Node) addEdge(e *Edge) {
	node.Edges = append(node.Edges, e)
}

func (node *Node) removeEdge(e *Edge) {
	for i, currE := range node.Edges {
		if currE == e {
			node.Edges = append(node.Edges[:i], node.Edges[i+1:]...)
			break
		}
	}
}

func (node *Node) isPointNear(point *geo.Point) bool {
	return (((node.TopLeft.X - MinRouteNodeClearance) <= point.X) && ((node.TopLeft.X + node.Width + MinRouteNodeClearance) >= point.X)) &&
		(((node.TopLeft.Y - MinRouteNodeClearance) <= point.Y) && ((node.TopLeft.Y + node.Height + MinRouteNodeClearance) >= point.Y))
}

func (node *Node) isPointOnNode(point *geo.Point) bool {
	return ((node.TopLeft.X <= point.X) && ((node.TopLeft.X + node.Width) >= point.X)) &&
		((node.TopLeft.Y <= point.Y) && ((node.TopLeft.Y + node.Height) >= point.Y))
}

func (nodeA *Node) distanceTo(nodeB *Node, includeSizes bool) float64 {
	return distanceBetweenBoxes(nodeDistanceBox(nodeA, includeSizes), nodeDistanceBox(nodeB, includeSizes))
}

func nodeDistanceBox(node *Node, includeSize bool) geo.Box {
	box := geo.Box{TopLeft: node.TopLeft}
	if includeSize {
		box.Width = node.Width
		box.Height = node.Height
	}
	return box
}

// distanceBetweenBoxes returns the shortest Euclidean distance between two
// closed, axis-aligned boxes. Overlapping and touching boxes have distance 0.
func distanceBetweenBoxes(b1, b2 geo.Box) float64 {
	dx := intervalGap(b1.TopLeft.X, b1.TopLeft.X+b1.Width, b2.TopLeft.X, b2.TopLeft.X+b2.Width)
	dy := intervalGap(b1.TopLeft.Y, b1.TopLeft.Y+b1.Height, b2.TopLeft.Y, b2.TopLeft.Y+b2.Height)
	return geo.EuclideanDistance(0, 0, dx, dy)
}

func intervalGap(aStart, aEnd, bStart, bEnd float64) float64 {
	if aEnd < bStart {
		return bStart - aEnd
	}
	if bEnd < aStart {
		return aStart - bEnd
	}
	return 0
}

func (nodeA *Node) isAdjacentTo(nodeB *Node, includeSizes bool) bool {
	d := nodeA.distanceTo(nodeB, includeSizes)
	if includeSizes {
		if d <= nodeA.Graph.CellSize {
			return true
		}
	} else {
		if d <= 1.0 {
			return true
		}
	}
	return false
}

func (node *Node) isWithinBounds(tl, br *geo.Point) bool {
	if node.TopLeft.X > br.X {
		return false
	}
	if node.TopLeft.Y > br.Y {
		return false
	}
	if node.TopLeft.X+node.Width < tl.X {
		return false
	}
	if node.TopLeft.Y+node.Height < tl.Y {
		return false
	}

	return true
}

func (nodeA *Node) distanceBetweenCenters(nodeB *Node, includeSizes bool) float64 {
	c := 1.0 / 20.0

	xCenter := math.Abs(nodeA.TopLeft.X - nodeB.TopLeft.X)
	yCenter := math.Abs(nodeA.TopLeft.Y - nodeB.TopLeft.Y)

	if includeSizes {
		nodeAWidth := nodeA.Width
		nodeAHeight := nodeA.Height
		nodeBWidth := nodeB.Width
		nodeBHeight := nodeB.Height

		xCenter = math.Abs((nodeA.TopLeft.X+nodeAWidth/2)-(nodeB.TopLeft.X+nodeBWidth/2)) / (nodeAWidth + nodeBWidth)
		yCenter = math.Abs((nodeA.TopLeft.Y+nodeAHeight/2)-(nodeB.TopLeft.Y+nodeBHeight/2)) / (nodeAHeight + nodeBHeight)
	}

	return c * math.Min(xCenter, yCenter)
}

func (node *Node) IsBlocked(nodeA, nodeB *Node, includeSizes, isHorizontal bool) bool {
	// Check that node is in front of nodeA and behind nodeB (in between)
	// ----------
	if isHorizontal {
		if includeSizes {
			if !((node.TopLeft.X >= (nodeA.TopLeft.X + nodeA.Width)) && ((node.TopLeft.X + node.Width) <= nodeB.TopLeft.X)) {
				return false
			}
		} else {
			if !((node.TopLeft.X >= nodeA.TopLeft.X) && (node.TopLeft.X <= nodeB.TopLeft.X)) {
				return false
			}
		}
	} else {
		if includeSizes {
			if !((node.TopLeft.Y >= (nodeA.TopLeft.Y + nodeA.Height)) && ((node.TopLeft.Y + node.Height) <= nodeB.TopLeft.Y)) {
				return false
			}
		} else {
			if !((node.TopLeft.Y >= nodeA.TopLeft.Y) && (node.TopLeft.Y <= nodeB.TopLeft.Y)) {
				return false
			}
		}
	}

	// Check that node blocks nodeA from nodeB
	// ----------
	if isHorizontal {
		/**
		                   +----+
		           +       |    |
		+----+     |       |    |
		|    |     |       |    |
		|    |     |       |    |
		|    |     |       |    |
		|    |     |       +----+
		|    |     +
		+----+

		+----+
		|    |    +
		|    |    |
		|    |    |   +----+
		|    |    |   |    |
		|    |    |   |    |
		+----+    |   |    |
		          |   |    |
		          +   |    |
		              +----+
		*/

		if includeSizes {
			if (node.TopLeft.Y <= math.Max(nodeA.TopLeft.Y, nodeB.TopLeft.Y)) &&
				((node.TopLeft.Y + node.Height) >= math.Min(nodeA.TopLeft.Y+nodeA.Height, nodeB.TopLeft.Y+nodeB.Height)) {
				return true
			}
		} else {
			if (node.TopLeft.Y <= math.Max(nodeA.TopLeft.Y, nodeB.TopLeft.Y)) &&
				(node.TopLeft.Y >= math.Min(nodeA.TopLeft.Y, nodeB.TopLeft.Y)) {
				return true
			}
		}
	} else {
		// Same as horizontal, just axis flipped
		if includeSizes {
			if (node.TopLeft.X <= math.Max(nodeA.TopLeft.X, nodeB.TopLeft.X)) &&
				((node.TopLeft.X + node.Width) >= math.Min(nodeA.TopLeft.X+nodeA.Width, nodeB.TopLeft.X+nodeB.Width)) {
				return true
			}
		} else {
			if (node.TopLeft.X <= math.Max(nodeA.TopLeft.X, nodeB.TopLeft.X)) &&
				(node.TopLeft.X >= math.Min(nodeA.TopLeft.X, nodeB.TopLeft.X)) {
				return true
			}
		}
	}

	return false
}

func (nodeA *Node) distance(nodeB *Node, includeSizes bool) float64 {
	return nodeA.distanceTo(nodeB, includeSizes) + nodeA.distanceBetweenCenters(nodeB, includeSizes)
}

// HAS TO BE DIAGONAL FROM EACH OTHER
func (nodeA *Node) orthogonalDistanceTo(nodeB *Node) (x float64, y float64) {
	rightA := nodeA.TopLeft.X + nodeA.Width
	bottomA := nodeA.TopLeft.Y + nodeA.Height
	rightB := nodeB.TopLeft.X + nodeB.Width
	bottomB := nodeB.TopLeft.Y + nodeB.Height

	if nodeA.TopLeft.X < nodeB.TopLeft.X {
		x = math.Max(0, nodeB.TopLeft.X-rightA)
	} else {
		x = math.Max(0, nodeA.TopLeft.X-rightB)
	}

	if nodeA.TopLeft.Y < nodeB.TopLeft.Y {
		y = math.Max(0, nodeB.TopLeft.Y-bottomA)
	} else {
		y = math.Max(0, nodeA.TopLeft.Y-bottomB)
	}

	return x, y
}

// 0 for not aligned at all
// 1 for fully aligned
// As a measure of alignment, 3 arrows are extended from the widest/tallest node
// Each hit is 0.33, scaled by size of node
// In this example,
// - the top node is widest
// - the middle node can only hit 1, so is 0.33*3 = 1
// - the bottom node can hit 2, so is 0.33*2 = 0.66
// - Total: 1.66/2 = 0.83
/*
//
//       ┌────────────────────────────┐
//       │                            │
//       │                            │
//       │     │         │        │   │
//       │     │         │        │   │
//       │     │         │        │   │
//       └─────┼─────────┼────────┼───┘
//             │         │        │
//             │      ┌──┼───┐    │
//             │      │  │   │    │
//             │      └──┼───┘    │
//             │         │        │
// ┌───────────┴──────┐  │        │
// │           ▼      │  ▼        ▼
// └──────────────────┘
*/
func (node *Node) center() *geo.Point {
	return geo.NewPoint(node.TopLeft.X+node.Width/2, node.TopLeft.Y+node.Height/2)
}

func (n1 *Node) overlapsAlongDimension(n2 *Node, isHorizontal, includeSizes bool) bool {
	if isHorizontal {
		if includeSizes {
			if n1.TopLeft.Y > (n2.TopLeft.Y + n2.Height) {
				return false
			}
			if (n1.TopLeft.Y + n1.Height) < n2.TopLeft.Y {
				return false
			}
		} else {
			if n1.TopLeft.Y > n2.TopLeft.Y {
				return false
			}
			if n1.TopLeft.Y < n2.TopLeft.Y {
				return false
			}
		}
	} else {
		if includeSizes {
			if n1.TopLeft.X > (n2.TopLeft.X + n2.Width) {
				return false
			}
			if (n1.TopLeft.X + n1.Width) < n2.TopLeft.X {
				return false
			}
		} else {
			if n1.TopLeft.X > n2.TopLeft.X {
				return false
			}
			if n1.TopLeft.X < n2.TopLeft.X {
				return false
			}
		}
	}
	return true
}

// isVisibilityGraphCandidate checks for nodes which are in between given parameters node and otherNode
// a = node
// c = otherNode
// b = visible candidate given the padding
// .          ┌─────┐
// .          │  b  │
// .          └─────┘  ──┬─
// .                     │
// .                     │ padding
// . ┌─────┐           ──┴─ ┌────┐
// . │     │                │    │
// . │  a  │                │ c  │
// . └─────┘                └────┘
func (node *Node) isVisibilityGraphCandidate(isHorizontal, checkSide, includeSizes bool, otherNode *Node, padding float64) bool {
	if isHorizontal {
		if checkSide && (node.TopLeft.X >= otherNode.TopLeft.X) {
			return false
		}
		if includeSizes {
			if node.TopLeft.Y > (otherNode.TopLeft.Y + otherNode.Height + padding) {
				return false
			}
			if (node.TopLeft.Y + node.Height + padding) < otherNode.TopLeft.Y {
				return false
			}
		} else {
			if node.TopLeft.Y > otherNode.TopLeft.Y {
				return false
			}
			if node.TopLeft.Y < otherNode.TopLeft.Y {
				return false
			}
		}
	} else {
		if checkSide && (node.TopLeft.Y >= otherNode.TopLeft.Y) {
			return false
		}
		if includeSizes {
			if node.TopLeft.X > (otherNode.TopLeft.X + otherNode.Width + padding) {
				return false
			}
			if (node.TopLeft.X + node.Width + padding) < otherNode.TopLeft.X {
				return false
			}
		} else {
			if node.TopLeft.X > otherNode.TopLeft.X {
				return false
			}
			if node.TopLeft.X < otherNode.TopLeft.X {
				return false
			}
		}
	}

	return true
}

func (node *Node) passesThrough(p1, p2 *geo.Point) bool {
	return segmentIntersectsBox(p1, p2, &node.Box)
}

// TODO: extract to `geo` package
func segmentIntersectsBox(p1, p2 *geo.Point, box *geo.Box) bool {
	if p1 == nil || p2 == nil || box == nil || box.TopLeft == nil {
		return false
	}

	left, right := box.TopLeft.X, box.TopLeft.X+box.Width
	if left > right {
		left, right = right, left
	}
	top, bottom := box.TopLeft.Y, box.TopLeft.Y+box.Height
	if top > bottom {
		top, bottom = bottom, top
	}
	// A NaN box boundary cannot contain a point or survive the clipping below.
	// Handle it explicitly so ordered comparisons can replace math.Min/Max in
	// the broad phase without changing their special-value behavior.
	if left != left || right != right || top != top || bottom != bottom {
		return false
	}

	// Reject a segment only when both endpoints lie outside the same box side.
	// This is the same bounding-box test without sorting the segment endpoints
	// or calling the general-purpose NaN-aware math.Min/Max helpers per blocker.
	if (p1.X < left && p2.X < left) || (p1.X > right && p2.X > right) ||
		(p1.Y < top && p2.Y < top) || (p1.Y > bottom && p2.Y > bottom) {
		return false
	}

	contains := func(p *geo.Point) bool {
		return left <= p.X && p.X <= right && top <= p.Y && p.Y <= bottom
	}
	if contains(p1) || contains(p2) {
		return true
	}

	// Clip the segment against the closed box. Both endpoints are outside here,
	// so a zero-length clipped interval is a single-point tangency rather than a
	// crossover. We allow that case, while a positive-length interval means the
	// segment crosses the box or overlaps one of its boundaries.
	tEnter, tExit := 0.0, 1.0
	clipAxis := func(start, delta, minCoord, maxCoord float64) bool {
		if delta == 0 {
			return minCoord <= start && start <= maxCoord
		}
		t1 := (minCoord - start) / delta
		t2 := (maxCoord - start) / delta
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tEnter = math.Max(tEnter, t1)
		tExit = math.Min(tExit, t2)
		return tEnter <= tExit
	}

	if !clipAxis(p1.X, p2.X-p1.X, left, right) ||
		!clipAxis(p1.Y, p2.Y-p1.Y, top, bottom) {
		return false
	}
	return tEnter < tExit
}

// orientation returns a positive value for a clockwise turn, a negative value
// for a counter-clockwise turn, and zero for collinear points.
func orientation(p, q, r *geo.Point) float64 {
	pqX := q.X - p.X
	pqY := q.Y - p.Y
	prX := r.X - p.X
	prY := r.Y - p.Y
	return pqY*prX - pqX*prY
}

func intersects(p1, q1, p2, q2 *geo.Point) bool {
	p2Side := orientation(p1, q1, p2)
	q2Side := orientation(p1, q1, q2)
	p1Side := orientation(p2, q2, p1)
	q1Side := orientation(p2, q2, q1)

	if p2Side == 0 && q2Side == 0 && p1Side == 0 && q1Side == 0 {
		return closedIntervalsOverlap(p1.X, q1.X, p2.X, q2.X) &&
			closedIntervalsOverlap(p1.Y, q1.Y, p2.Y, q2.Y)
	}

	return straddlesLine(p2Side, q2Side) && straddlesLine(p1Side, q1Side)
}

func straddlesLine(side1, side2 float64) bool {
	return side1 == 0 || side2 == 0 || (side1 < 0) != (side2 < 0)
}

func closedIntervalsOverlap(a1, a2, b1, b2 float64) bool {
	aMin, aMax := math.Min(a1, a2), math.Max(a1, a2)
	bMin, bMax := math.Min(b1, b2), math.Max(b1, b2)
	return aMin <= bMax && bMin <= aMax
}

// return true if the point is within the node's bounding box with delta padding
// Example: A: false; B: true; C: true; D: false; E: true
// delta       delta
// ├──┤  A     ├──┤
// ┌──────────────┐ ┬
// │ B            │ │ delta
// │   ┌─node─┐   │ ┴
// │   │  C   │   │   D
// │   └──────┘   │ ┬
// │              │ │ delta
// └───────────E──┘ ┴
func (node *Node) containsPoint(p *geo.Point, delta float64) bool {
	return node.TopLeft.X-delta <= p.X &&
		node.TopLeft.X+node.Width+delta >= p.X &&
		node.TopLeft.Y-delta <= p.Y &&
		node.TopLeft.Y+node.Height+delta >= p.Y
}

// return true if the line overlaps the node bounding box
func (node *Node) overlapsLine(p1, p2 *geo.Point, delta float64) bool {
	if node.containsPoint(p1, delta) || node.containsPoint(p2, delta) {
		return true
	}

	l := node.TopLeft.X - delta
	r := node.TopLeft.X + node.Width + delta
	t := node.TopLeft.Y - delta
	b := node.TopLeft.Y + node.Height + delta

	tl := geo.NewPoint(l, t)
	br := geo.NewPoint(r, b)
	tr := geo.NewPoint(r, t)
	bl := geo.NewPoint(l, b)

	return intersects(tl, tr, p1, p2) ||
		intersects(tr, br, p1, p2) ||
		intersects(br, bl, p1, p2) ||
		intersects(bl, tl, p1, p2)
}

func (node *Node) Surrounds(other *Node, withPadding float64) bool {
	if node == nil {
		return true
	}
	return node.TopLeft.X+withPadding < other.TopLeft.X &&
		node.TopLeft.X+node.Width-withPadding > other.TopLeft.X+other.Width &&
		node.TopLeft.Y+withPadding < other.TopLeft.Y &&
		node.TopLeft.Y+node.Height-withPadding > other.TopLeft.Y+other.Height
}

// orientation returns the orientation of node relative to otherNode.
// E.g. node ---> otherNode, here, node is to the left of otherNode, so Left would be returned
func (node *Node) orientation(otherNode *Node) geo.Orientation {
	if node.TopLeft == nil || otherNode.TopLeft == nil {
		return geo.NONE
	}
	if (node.TopLeft.Y + node.Height) < otherNode.TopLeft.Y {
		if (node.TopLeft.X + node.Width) < otherNode.TopLeft.X {
			return geo.TopLeft
		}
		if (otherNode.TopLeft.X + otherNode.Width) < node.TopLeft.X {
			return geo.TopRight
		}
		return geo.Top
	}

	if (otherNode.TopLeft.Y + otherNode.Height) < node.TopLeft.Y {
		if (node.TopLeft.X + node.Width) < otherNode.TopLeft.X {
			return geo.BottomLeft
		}
		if (otherNode.TopLeft.X + otherNode.Width) < node.TopLeft.X {
			return geo.BottomRight
		}
		return geo.Bottom
	}

	if (otherNode.TopLeft.X + otherNode.Width) < node.TopLeft.X {
		return geo.Right
	}

	if (node.TopLeft.X + node.Width) < otherNode.TopLeft.X {
		return geo.Left
	}

	return geo.NONE
}

func (node *Node) orientationAtPoint(otherNode *Node, point *geo.Point) geo.Orientation {
	// This is an optimized version that doesn't temporarily modify node's position
	if point == nil || otherNode.TopLeft == nil {
		return geo.NONE
	}

	// Simulate node being at point instead of its actual position
	// Same logic as orientation but with point coordinates instead of node.TopLeft
	if (point.Y + node.Height) < otherNode.TopLeft.Y {
		if (point.X + node.Width) < otherNode.TopLeft.X {
			return geo.TopLeft
		}
		if (otherNode.TopLeft.X + otherNode.Width) < point.X {
			return geo.TopRight
		}
		return geo.Top
	}

	if (otherNode.TopLeft.Y + otherNode.Height) < point.Y {
		if (point.X + node.Width) < otherNode.TopLeft.X {
			return geo.BottomLeft
		}
		if (otherNode.TopLeft.X + otherNode.Width) < point.X {
			return geo.BottomRight
		}
		return geo.Bottom
	}

	if (otherNode.TopLeft.X + otherNode.Width) < point.X {
		return geo.Right
	}

	if (point.X + node.Width) < otherNode.TopLeft.X {
		return geo.Left
	}

	return geo.NONE
}

// allReachableNodesGuarded returns all connected nodes in BFS order
// This includes itself at index 0
func (node *Node) allReachableNodesGuarded(
	includeContainers, includeNears, traverseTrees bool,
	ignore map[*Node]struct{},
	guard workStepper,
) ([]*Node, error) {
	return node.reachableNodesGuarded(
		func(_ *Node) bool { return true },
		includeContainers,
		includeNears,
		traverseTrees,
		ignore,
		guard,
	)
}

// reachabilityIsDescendantOf preserves isDescendantOf's ancestry precedence
// while charging every parent hop. Container reachability compares many node
// pairs, so charging only the pair and not these walks leaves a deep hierarchy
// able to bypass the operation's aggregate budget.
func reachabilityIsDescendantOf(
	maybeDescendant, maybeAncestor *Node,
	guard workStepper,
) (bool, error) {
	if guard == nil {
		return false, fmt.Errorf("TALA reachability ancestry requires a work guard")
	}
	for current := maybeDescendant; ; current = ancestryParent(current) {
		if err := guard.Step(); err != nil {
			return false, err
		}
		if maybeAncestor == current {
			return true, nil
		}
		if current == nil {
			return false, nil
		}
	}
}

func (node *Node) reachableNodesGuarded(
	shouldVisit func(*Node) bool,
	includeContainers, includeNears, traverseTrees bool,
	ignore map[*Node]struct{},
	guard workStepper,
) ([]*Node, error) {
	reachableNodes := make([]*Node, 0)
	visitQueue := []*Node{node}

	reachedOrVisited := map[*Node]struct{}{
		node: {},
	}

	queue := func(n *Node) {
		if _, ok := ignore[n]; ok {
			return
		}
		if _, ok := reachedOrVisited[n]; ok {
			return
		}
		if !shouldVisit(n) {
			return
		}
		reachedOrVisited[n] = struct{}{}
		visitQueue = append(visitQueue, n)
	}

	for len(visitQueue) > 0 {
		if err := guard.Step(); err != nil {
			return nil, err
		}
		curr := visitQueue[0]
		visitQueue = visitQueue[1:]
		includeNode := true
		if traverseTrees {
			if _, is := node.Graph.NodeToTree[curr]; is {
				includeNode = false
			}
		}
		if includeNode {
			reachableNodes = append(reachableNodes, curr)
		}
		reachedOrVisited[curr] = struct{}{}

		for _, e := range curr.Edges {
			if err := guard.Step(); err != nil {
				return nil, err
			}
			adjacentNode := curr.adjacent(e)
			queue(adjacentNode)
		}

		if traverseTrees {
			if trees, isSentinel := node.Graph.Trees[curr]; isSentinel {
				for _, tree := range trees {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					queue(tree.Node)
				}
			} else if tree, is := node.Graph.NodeToTree[curr]; is {
				if tree.Parent != nil {
					queue(tree.Parent.Node)
				} else {
					queue(tree.sentinelNode())
				}
				for _, c := range tree.Children {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					queue(c.Node)
				}
			}
		}
		if includeNears {
			nears := curr.orderedNears()
			if curr.isClusterVessel {
				for _, cn := range node.Graph.Clusters[curr].Nodes {
					nears = append(nears, cn.orderedNears()...)
				}
			}
			for _, near := range nears {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				if near.Cluster.isActive() {
					near = near.Cluster.Vessel
				} else if near.Sequence.isActive() {
					near = near.Sequence.Vessel
				}
				if near.Container != node.Container {
					continue
				}
				queue(near)
			}
		}

		if includeContainers {
			if curr.Sequence != nil {
				for _, step := range curr.Sequence.Nodes {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					queue(step)
				}
			}
			if curr.isClusterVessel {
				for _, node := range node.Graph.Clusters[curr].Nodes {
					if err := guard.Step(); err != nil {
						return nil, err
					}
					queue(node)
				}
			}

			for _, otherNode := range node.Graph.Nodes {
				if err := guard.Step(); err != nil {
					return nil, err
				}
				if _, in := reachedOrVisited[otherNode]; in {
					continue
				}
				isDescendant, err := reachabilityIsDescendantOf(curr, otherNode, guard)
				if err != nil {
					return nil, err
				}
				if !isDescendant {
					isDescendant, err = reachabilityIsDescendantOf(otherNode, curr, guard)
					if err != nil {
						return nil, err
					}
				}
				if isDescendant {
					queue(otherNode)
				}
			}
		}
	}
	return reachableNodes, nil
}

func (node *Node) connectionTo(otherNode *Node) *Edge {
	for _, e := range node.Edges {
		if node.adjacent(e) == otherNode {
			return e
		}
	}
	return nil
}

// spillsOutOfNode detects whether the node has both
// a corner inside the container and a corner outside the container node
func (node1 *Node) spillsOutOf(node2 *Node) bool {
	corners := []*geo.Point{
		node1.TopLeft,
		geo.NewPoint(node1.TopLeft.X+node1.Width, node1.TopLeft.Y),
		geo.NewPoint(node1.TopLeft.X+node1.Width, node1.TopLeft.Y+node1.Height),
		geo.NewPoint(node1.TopLeft.X, node1.TopLeft.Y+node1.Height),
	}

	hasInside := false
	hasOutside := false

	for _, corner := range corners {
		if node2.isPointOnNode(corner) {
			hasInside = true
		} else {
			hasOutside = true
		}
	}

	return hasInside && hasOutside
}

// connectedNodes returns all nodes (including this one) that should move with it.
// This includes nodes with a path to it, nodes in the same container
// TODO for some reason e.From.Graph isn't populated with containers, so we pass mainGraph instead
func (node *Node) connectedNodes(excludedNodes []*Node, mainGraph *Graph) []*Node {
	nodes := make([]*Node, 0)
	queue := []*Node{node}
	inQueue := map[*Node]bool{node: true}

	excludedSet := make(map[*Node]bool)
	for _, n := range excludedNodes {
		excludedSet[n] = true
	}

	addQueue := func(n *Node) {
		if _, in := inQueue[n]; !in {
			queue = append(queue, n)
			inQueue[n] = true
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		isAncestorOrDescendantOfExcluded := false
		for _, excluded := range excludedNodes {
			if curr.isDescendantOf(excluded) {
				isAncestorOrDescendantOfExcluded = true
				break
			}
			if excluded.isDescendantOf(curr) && !node.isDescendantOf(curr) {
				isAncestorOrDescendantOfExcluded = true
				break
			}
		}
		if isAncestorOrDescendantOfExcluded {
			continue
		}
		nodes = append(nodes, curr)
		for _, e := range curr.Edges {
			connected := curr.adjacent(e)
			if _, is := excludedSet[connected]; is {
				continue
			}
			addQueue(connected)
		}

		// If the current is a cluster vessel, a cluster node may be a container, which we should include the children of
		if curr.isClusterVessel {
			for _, n := range mainGraph.Clusters[curr].Nodes {
				if n.isContainer {
					for _, child := range mainGraph.Containers[n] {
						addQueue(child)
					}
				}
			}
		}

		// If the current is a child of a cluster node container, we should include that cluster vessel
		for vessel, cluster := range mainGraph.Clusters {
			for _, n := range cluster.Nodes {
				if n.isContainer {
					nodeIsChild := false
					excludedNodeIsChild := false
					for _, child := range mainGraph.Containers[n] {
						if child == curr {
							nodeIsChild = true
						}
						if _, is := excludedSet[child]; is {
							excludedNodeIsChild = true
							break
						}
					}
					if excludedNodeIsChild {
						continue
					}
					if nodeIsChild {
						addQueue(vessel)
					}
				}
			}
		}

		for container := range mainGraph.Containers {
			if container == nil {
				continue
			}
			if _, is := excludedSet[container]; is {
				continue
			}

			// If the node is part a container, include the container (which will include all children downstream) ...
			nodeIsChild := false
			excludedNodeIsChild := false

			for _, child := range mainGraph.Containers[container] {
				if child == curr {
					nodeIsChild = true
				}
				if _, is := excludedSet[child]; is {
					excludedNodeIsChild = true
					break
				}
			}
			// ... unless an excluded node is also within that container
			if excludedNodeIsChild {
				continue
			}
			if nodeIsChild {
				addQueue(container)
			}

			// If the node is a container, everything within it is included
			if curr == container {
				for _, child := range mainGraph.Containers[curr] {
					addQueue(child)
				}
			}
		}
	}

	return nodes
}

func (node *Node) area() float64 {
	return node.Width * node.Height
}

// LabelTopLeft returns the top-left point for a label of the given size and
// position relative to node.
func (node *Node) LabelTopLeft(labelPosition label.Position, width, height float64) *geo.Point {
	var box *geo.Box
	if labelPosition.IsOutside() {
		box = node.GetBox()
	} else {
		box = node.InnerBox()
	}

	return labelPosition.GetPointOnBox(box, label.PADDING, width, height)
}

// NodeOverlapCount reports how many nodes overlap node within delta.
func (node *Node) NodeOverlapCount(nodes Nodes, delta float64) int {
	count := 0
	for _, otherNode := range nodes {
		if node.doesOverlapCalc(otherNode, delta) {
			count++
		}
	}
	return count
}

func (n Node) ports() []geo.Point {
	ports := make([]geo.Point, 0, 12)
	for _, relativePoints := range n.SnapPointPercentages() {
		for _, point := range relativePoints {
			ports = append(ports, *geo.NewPoint(n.TopLeft.X+math.Round(n.Width*point.XPercentage), n.TopLeft.Y+math.Round(n.Height*point.YPercentage)))
		}
	}
	return ports
}

func (n Node) nthPortValue(index int) (geo.Point, bool) {
	i := 0
	for _, relativePoints := range n.SnapPointPercentages() {
		for _, point := range relativePoints {
			if i == index {
				return geo.Point{
					X: n.TopLeft.X + math.Round(n.Width*point.XPercentage),
					Y: n.TopLeft.Y + math.Round(n.Height*point.YPercentage),
				}, true
			}
			i++
		}
	}
	return geo.Point{}, false
}

func (n Node) nthPort(index int) *geo.Point {
	port, ok := n.nthPortValue(index)
	if !ok {
		return nil
	}
	return &port
}

func (node Node) pointToPortOrientation(p *geo.Point) geo.Orientation {
	for _, o := range []geo.Orientation{geo.Top, geo.Right, geo.Left, geo.Bottom} {
		indices := node.PortIndices(o)
		for _, i := range indices {
			port := node.nthPort(i)
			if p.Equals(port) {
				return o
			}
		}
	}
	return geo.NONE
}

func (node Node) nthPortOnSideValue(orientation geo.Orientation, n int) (geo.Point, bool) {
	indices := node.PortIndices(orientation)
	return node.nthPortValue(indices[n])
}

func (node Node) tableColumnPortValue(orientation geo.Orientation, columnIndex int) (geo.Point, bool) {
	if port, ok := nodeshape.TableColumnPortValue(node.Shape, orientation, columnIndex); ok {
		return port, true
	}
	return node.nthPortOnSideValue(orientation, columnIndex)
}

func (n Node) centerPorts() []geo.Point {
	ports := n.ports()
	indices := n.CenterPortIndices()

	centerPorts := make([]geo.Point, 0, len(indices))
	for _, index := range indices {
		centerPorts = append(centerPorts, ports[index])
	}
	return centerPorts
}

func (n Node) portsByOrientation(orientation geo.Orientation) []geo.Point {
	ports := n.ports()
	indices := n.PortIndices(orientation)

	portsForOrientation := make([]geo.Point, 0, len(indices))
	for _, index := range indices {
		portsForOrientation = append(portsForOrientation, ports[index])
	}
	return portsForOrientation
}

func (n Node) mirroredPorts() map[geo.Point]geo.Point {
	ports := n.ports()

	portToMirrored := make(map[geo.Point]geo.Point)
	for index, mirrorIndex := range n.MirroredPortIndices() {
		portToMirrored[ports[index]] = ports[mirrorIndex]
	}
	return portToMirrored
}

func (node *Node) mirror(x, y bool) {
	// When mirroring, we need to consider the loop offsets
	// otherwise, the node bounding box can change can end up in the wrong position.
	// In the example below, note how before mirroring, both nodes have their bounding boxes aligned at the same Y coodinate.
	// After mirroring, the node with a loop offset got a different Y coodinate misaligning the nodes.
	// In order to get the right bounding box after mirroring, it needs to account for the given offset
	// y = node.TopLeft.Y
	// h = node.Height
	// after mirror
	//         ┌────────┐
	//         │        │
	//     ┬   │   ┌────┴───────┐      ┌────────────────┐
	//     |   │   │            │      │                │
	//  -h |   └───►            │      │                │
	//     |       │            │      │                │
	//     ┴      -y────────────┘      │                │
	//                                 │                │
	//                                 └────────────────┘
	// ───────────────────────────────────────────────────────────── mirror axis
	// before mirror
	//         ┌────────┐              ┌────────────────┐
	//         │        │              │                │
	//     ┬   │   y────┴───────┐      │                │
	//     |   │   │            │      │                │
	//   h |   └───►            │      │                │
	//     |       │            │      │                │
	//     ┴       └────────────┘      └────────────────┘
	// the same applies when mirroring over the X axis
	var topOffset, bottomOffset, leftOffset, rightOffset float64
	if len(node.LoopOffsets) > 0 {
		topOffset = node.LoopOffsets[geo.Top]
		leftOffset = node.LoopOffsets[geo.Left]
		rightOffset = node.LoopOffsets[geo.Right]
		bottomOffset = node.LoopOffsets[geo.Bottom]
	}
	if x {
		node.TopLeft.X = -node.TopLeft.X - node.Width + leftOffset - rightOffset
	}
	if y {
		node.TopLeft.Y = -node.TopLeft.Y - node.Height + topOffset - bottomOffset
	}
}

func (n *Node) moveNodeAbsWithChildren(x, y float64) {
	if n.TopLeft != nil && n.TopLeft.X == x && n.TopLeft.Y == y {
		return
	}
	n.moveNodeWithChildren(x-n.TopLeft.X, y-n.TopLeft.Y)
}

func (n *Node) moveNodeWithChildren(dx, dy float64) {
	if dx == 0 && dy == 0 {
		return
	}
	n.translate(dx, dy)
	for _, child := range n.Graph.allDescendantNodes(n, true) {
		child.translate(dx, dy)
	}
}

func (n *Node) translate(dx, dy float64) {
	n.TopLeft.X += dx
	n.TopLeft.Y += dy
}

func (n *Node) positionContainerChildren(withPadding bool) {
	var children Nodes
	if !n.isContainer {
		return
	}
	children = n.Graph.Containers[n]
	var padding Spacing
	if withPadding {
		padding = n.Graph.containerPadding(n, false)
	}
	tl, br := children.fixedBounds()
	n.expandForLabels(tl, br)
	innerTL := n.InsidePlacement(br.X-tl.X, br.Y-tl.Y, padding)
	dx := innerTL.X - tl.X
	dy := innerTL.Y - tl.Y
	for _, childN := range children {
		childN.moveNodeWithChildren(dx, dy)
	}
}

func (n Node) CanContain() bool {
	return n.shapeType != imageType &&
		n.shapeType != codeType &&
		n.shapeType != tableType &&
		n.shapeType != classType &&
		n.shapeType != textType
}

// Make enough room for label to be center positioned
// Even though labels are placed at the last step, the most common is for long labels to be outside centered top or bottom.
// So we pre-emptively make enough room for it.
func (n *Node) expandForLabels(tl, br *geo.Point) {
	children := n.Graph.Containers[n]

	for _, child := range children {
		if child.Label != nil && (child.TopLeft.X == tl.X || child.TopLeft.X+child.Width == br.X) {
			if child.Label.Width > child.Width {
				tl.X = math.Min(tl.X, math.Floor(child.TopLeft.X+(child.Width/2.-child.Label.Width/2.)))
				br.X = math.Max(br.X, math.Ceil(child.TopLeft.X+child.Width-(child.Width/2.-child.Label.Width/2.)))
			}
		}
	}
}

func (n *Node) wrapChildren() {
	var children Nodes
	if !n.isContainer {
		return
	}
	children = n.Graph.Containers[n]
	padding := n.Graph.containerPadding(n, false)
	tl, br := children.fixedBounds()
	n.expandForLabels(tl, br)
	n.fitToBoundingBox(tl, br, padding)
	innerTL := n.InsidePlacement(br.X-tl.X, br.Y-tl.Y, padding)
	n.translate(tl.X-innerTL.X, tl.Y-innerTL.Y)
}

// InsidePlacement returns the top-left point at which content of the given
// size and padding fits inside n.
func (n *Node) InsidePlacement(width, height float64, padding Spacing) geo.Point {
	padX := padding.left + padding.right
	padY := padding.top + padding.bottom
	// TODO update d2's GetInsidePlacement to take Spacing
	p := n.Shape.GetInsidePlacement(width, height, padX, padY)

	if n.Shape.GetType() == shape.CIRCLE_TYPE {
		totalWidth := width + padX
		totalHeight := height + padY

		innerBox := n.Shape.GetInnerBox()
		if innerBox.Width > totalWidth {
			p.X += (innerBox.Width - totalWidth) / 2.
		}
		if innerBox.Height > totalHeight {
			p.Y += (innerBox.Height - totalHeight) / 2.
		}
	}

	// always round the results from shape
	p.X = math.Round(p.X)
	p.Y = math.Round(p.Y)
	p.X -= math.Round(padX/2) - padding.left
	p.Y -= math.Round(padY/2) - padding.top
	return p
}

// InnerBox returns n's rounded content box.
func (n *Node) InnerBox() *geo.Box {
	// always round the results from shape
	box := n.Shape.GetInnerBox()
	return &geo.Box{
		TopLeft: &geo.Point{
			X: math.Round(box.TopLeft.X),
			Y: math.Round(box.TopLeft.Y),
		},
		Width:  math.Round(box.Width),
		Height: math.Round(box.Height),
	}
}

func (n *Node) isMajorityTarget() bool {
	var counter int
	for _, e := range n.Edges {
		if e.isDirected() {
			if e.isTargetedTo(n) {
				counter++
			} else {
				counter--
			}
		}
	}
	return counter > 0
}

// IconSize returns the icon size appropriate for iconPosition on n.
func (n *Node) IconSize(iconPosition label.Position) float64 {
	minDimension := math.Min(n.Width, n.Height)
	var size float64
	if iconPosition == label.InsideMiddleCenter {
		size = 0.5 * minDimension
	} else {
		size = math.Min(minDimension, math.Max(defaultIconSize, 0.5*minDimension))
	}
	size = math.Min(size, MaxIconSize)

	if !iconPosition.IsOutside() {
		box := n.InnerBox()
		size = math.Min(math.Max(0, box.Width-2*label.PADDING), size)
		size = math.Min(math.Max(0, box.Height-2*label.PADDING), size)
	}
	return size
}

func (n *Node) orderedNears() []*Node {
	var nears []*Node
	for near := range n.Nears {
		nears = append(nears, near)
	}
	sortNodesByID(nears)
	return nears
}

func (n *Node) dfsWalk(applyFunc func(*Node) (stop bool)) bool {
	if applyFunc(n) {
		// stop early if apply returns true
		return true
	}

	if n.isContainer {
		for _, childNode := range n.Graph.Containers[n] {
			if childNode.dfsWalk(applyFunc) {
				return true
			}
		}
	}

	if n.isClusterVessel {
		for _, cn := range n.Graph.Clusters[n].Nodes {
			if cn.dfsWalk(applyFunc) {
				return true
			}
		}
	} else if s, is := n.Graph.Sequences[n]; is {
		for _, step := range s.Nodes {
			if step.dfsWalk(applyFunc) {
				return true
			}
		}
	}
	return false
}

func (n *Node) rdfsWalk(applyFunc func(*Node)) {
	if n.isContainer {
		for _, childNode := range n.Graph.Containers[n] {
			childNode.rdfsWalk(applyFunc)
		}
	}

	if n.isClusterVessel {
		for _, cn := range n.Graph.Clusters[n].Nodes {
			cn.rdfsWalk(applyFunc)
		}
	} else if s, is := n.Graph.Sequences[n]; is {
		for _, step := range s.Nodes {
			step.rdfsWalk(applyFunc)
		}
	}

	applyFunc(n)
}

func (n *Node) container() *Node {
	if n.Cluster.isActive() {
		return n.Cluster.Vessel.Container
	} else if n.Sequence.isActive() {
		return n.Sequence.Vessel.Container
	} else {
		return n.Container
	}
}

func (n *Node) containerLevel() int {
	level := 0
	for curr := n; curr != nil; curr = curr.container() {
		level++
	}
	return level
}

// returns the most nested container that is an ancestor of both n and otherNode
// In this example X.nearestSharedAncestor(Y) returns B.
// C, D, and E are nearer to the nodes X and Y, but are not a shared ancestor.
// A and nil are shared ancestors, but are further than B.
// ┌A───────────────────────────────────────────┐
// │  ┌B─────────────────────────────────────┐  │
// │  │                ┌D─────────────────┐  │  │
// │  │  ┌C─────────┐  │                  │  │  │
// │  │  │          │  │  ┌E───────────┐  │  │  │
// │  │  │  ┌────┐  │  │  │  ┌──────┐  │  │  │  │
// │  │  │  │ X  │  │  │  │  │ Y    │  │  │  │  │
// │  │  │  │    │  │  │  │  │      │  │  │  │  │
// │  │  │  └────┘  │  │  │  └──────┘  │  │  │  │
// │  │  │          │  │  └────────────┘  │  │  │
// │  │  └──────────┘  │                  │  │  │
// │  │                └──────────────────┘  │  │
// │  └──────────────────────────────────────┘  │
// └────────────────────────────────────────────┘
func (n *Node) nearestSharedAncestor(otherNode *Node) *Node {
	container := n.container()
	otherContainer := otherNode.container()
	if container == nil || otherContainer == nil {
		return nil
	}
	nLevel := n.containerLevel()
	otherLevel := otherNode.containerLevel()

	for container != otherContainer {
		if nLevel == otherLevel {
			container = container.container()
			nLevel--
			otherContainer = otherContainer.container()
			otherLevel--
		} else if nLevel > otherLevel {
			container = container.container()
			nLevel--
		} else {
			otherContainer = otherContainer.container()
			otherLevel--
		}
		if container == nil || otherContainer == nil {
			return nil
		}
	}
	return container
}

func (n *Node) transpose() {
	n.Width, n.Height = n.Height, n.Width
	n.TopLeft.Transpose()
	n.padding.left, n.padding.top = n.padding.top, n.padding.left
	n.padding.right, n.padding.bottom = n.padding.bottom, n.padding.right
	n.margin.left, n.margin.top = n.margin.top, n.margin.left
	n.margin.right, n.margin.bottom = n.margin.bottom, n.margin.right
}

// ContainerDirection reports the layout direction of n's owning container.
func (n *Node) ContainerDirection() geo.Orientation {
	return n.Graph.Direction(n.container())
}

func (n *Node) pad(padding int) {
	pad := float64(padding)
	n.translate(-pad, -pad)
	n.Width += 2 * pad
	n.Height += 2 * pad
}

func (n *Node) modifierElementAdjustments() (dx, dy float64) {
	if n.Is3D {
		if n.shapeType == hexagonType {
			dy = threeDOffset / 2
		} else {
			dy = threeDOffset
		}
		dx = threeDOffset
	} else if n.IsMultiple {
		dy = multipleOffset
		dx = multipleOffset
	}
	return dx, dy
}
