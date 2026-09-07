package engine

import (
	"context"
	"math"
	"math/rand"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

const (
	minFuzzNodeSize              = 50
	maxFuzzNodeSize              = 750
	maxFuzzNodePlacementAttempts = 5
	fuzzContainerProbability     = 0.5
	fuzzTreeProbability          = 0.1
	maxFuzzTreeDepth             = 10
	fuzzArrowheadProbability     = 0.5
)

var fuzzShapes = []string{
	shape.SQUARE_TYPE,
	shape.REAL_SQUARE_TYPE,
	shape.PARALLELOGRAM_TYPE,
	shape.DOCUMENT_TYPE,
	shape.CYLINDER_TYPE,
	shape.QUEUE_TYPE,
	shape.PAGE_TYPE,
	shape.PACKAGE_TYPE,
	shape.STEP_TYPE,
	shape.CALLOUT_TYPE,
	shape.STORED_DATA_TYPE,
	shape.PERSON_TYPE,
	shape.DIAMOND_TYPE,
	shape.OVAL_TYPE,
	shape.CIRCLE_TYPE,
	shape.HEXAGON_TYPE,
	shape.CLOUD_TYPE,
}

type graphFuzzerConfig struct {
	maxNodes                      int
	connectionIterations          int
	compactionFactor              float64
	nodeSubsetPercentageToConnect float64
	minConnectionProbability      float64
	maxConnectionProbability      float64
	seed                          int64
}

func createRandomGraph(ctx context.Context, config graphFuzzerConfig) *layoutgraph.Graph {
	// nolint: gosec
	random := rand.New(rand.NewSource(config.seed))
	g := layoutgraph.NewGraph()
	nNodes := int(randomInBetween(random, float64(config.maxNodes)/2.0, float64(config.maxNodes)))
	maxNodeSize := math.Min(maxFuzzNodeSize, float64(maxGraphSize)/float64(nNodes))
	minNodeSize := math.Max(minFuzzNodeSize, math.Sqrt(maxNodeSize))
	maxLength := computeMaxGraphSideLength(config, nNodes, maxNodeSize)
	treeRoots := make(map[*layoutgraph.Node]struct{})

	addNodes(
		random,
		nNodes,
		treeRoots,
		g,
		nil,
		geo.NewPoint(0, 0),
		geo.NewPoint(float64(maxLength), float64(maxLength)),
		minNodeSize,
		maxNodeSize,
	)

	for i := 0; i < config.connectionIterations; i++ {
		connectRandomNodes(random, config, g, treeRoots, maxLength)
	}

	return g
}

// assuming a perfect square graph, this is estimate of the maximum side length
func computeMaxGraphSideLength(config graphFuzzerConfig, nNodes int, maxNodeSize float64) (length int) {
	maxTopLeft := maxGraphSize - maxFuzzNodeSize
	length = nNodes * int(maxNodeSize)   // single row/column wihtout any spacing
	length = int(float64(length) * 1.25) // add 1/4 for spacing between nodes
	// if we fill in both directions, sqrt(length) would be the max size
	// however, as it's random, give it some room to place nodes at random, so just cut it to 3/4
	length = min(int(float64(length)*config.compactionFactor),
		// ensure it won't go outside our graph limit
		maxTopLeft)
	return length
}

func addNodes(random *rand.Rand, nNodes int, treeRoots map[*layoutgraph.Node]struct{}, g *layoutgraph.Graph, container *layoutgraph.Node, tl, br *geo.Point, minNodeSize, maxNodeSize float64) {
	for nNodes > 0 {
		nNodes--
		node := addNodeInRandomPlace(random, g, container, tl, br, minNodeSize, maxNodeSize)
		isContainer := random.Float64() <= fuzzContainerProbability
		hasContainerSize := math.Min(node.Width, node.Height) > minFuzzNodeSize
		nDescendants := int(math.Min(node.Width, node.Height)/minFuzzNodeSize) - 1
		nDescendants = int(math.Min(float64(nDescendants), float64(nNodes)))
		if isContainer && hasContainerSize && nNodes > 0 && nDescendants > 0 {
			nDescendants = int(randomInBetween(random, 1, float64(nDescendants)))
			maxChildrenSize := math.Min(node.Width, node.Height) / float64(nDescendants)
			addNodes(
				random,
				nDescendants,
				treeRoots,
				g,
				node,
				node.TopLeft,
				geo.NewPoint(node.TopLeft.X+node.Width-maxChildrenSize, node.TopLeft.Y+node.Height-maxChildrenSize),
				maxChildrenSize*0.75,
				maxChildrenSize,
			)
			nNodes -= nDescendants
		} else if random.Float64() <= fuzzTreeProbability {
			nTreeNodes := makeTree(
				random,
				node,
				nNodes,
				g,
				tl,
				br,
				minNodeSize,
				maxNodeSize,
			)
			nNodes -= nTreeNodes
			treeRoots[node] = struct{}{}
		}
	}
}

func makeTree(random *rand.Rand, root *layoutgraph.Node, maxNodes int, g *layoutgraph.Graph, tl, br *geo.Point, minNodeSize, maxNodeSize float64) int {
	// creates a tree layer by layer...
	treeNodes := 0
	depth := int(randomInBetween(random, 1, maxFuzzTreeDepth))
	previousLayer := []*layoutgraph.Node{root}
	for i := 0; i < depth; i++ {
		remainingNodes := float64(maxNodes) - float64(treeNodes)
		layerNodes := int(randomInBetween(random, 0, remainingNodes/float64(maxFuzzTreeDepth)))
		treeNodes += layerNodes
		if layerNodes == 0 {
			break
		}

		var newLayer []*layoutgraph.Node
		for j := 0; j < layerNodes; j++ {
			node := addNodeInRandomPlace(random, g, root.Container, tl, br, minNodeSize, maxNodeSize)
			newLayer = append(newLayer, node)
			connectTo := int(randomInBetween(random, 0, float64(len(previousLayer)-1)))
			e := g.Connect(previousLayer[connectTo], node)
			e.ID = layoutgraph.EntityID(len(g.Edges))
			if random.Float32() <= fuzzArrowheadProbability {
				e.TargetArrowhead = layoutgraph.TriangleArrowhead
			}
		}
		previousLayer = newLayer
	}
	return treeNodes
}

func addNodeInRandomPlace(random *rand.Rand, g *layoutgraph.Graph, container *layoutgraph.Node, tl, br *geo.Point, minNodeSize, maxNodeSize float64) *layoutgraph.Node {
	width := randomInBetween(random, minNodeSize, maxNodeSize)
	height := randomInBetween(random, minNodeSize, maxNodeSize)
	node := layoutgraph.NewNode(layoutgraph.EntityID(len(g.Nodes)+1), math.Trunc(width), math.Trunc(height))
	node.SetShape(fuzzShapes[int(width)%len(fuzzShapes)])

	// It tries to find a good position, if not possible leave it with overlap
	for i := 0; i < maxFuzzNodePlacementAttempts; i++ {
		node.TopLeft = geo.NewPoint(randomInBetween(random, tl.X, br.X),
			randomInBetween(random, tl.Y, br.Y),
		)
		placed := g.AddNode(node)
		if placed.ID == node.ID {
			g.AddNodeToContainer(container, node)
			// node was placed in a unique place, no overlap
			break
		}
	}

	return node
}

// random from [0, max - min] + min gives [min, max]
func randomInBetween(random *rand.Rand, min, max float64) float64 {
	if int(max-min) == 0 {
		// 0 < difference < 1
		return min
	}
	// Intn returns the max of an open interval, so +1 to return between [min, max]
	return float64(random.Intn(int(max-min+1))) + min
}

func connectRandomNodes(random *rand.Rand, config graphFuzzerConfig, g *layoutgraph.Graph, treeRoots map[*layoutgraph.Node]struct{}, maxLength int) {
	nodesToSkip := make(map[*layoutgraph.Node]struct{})
	indices := random.Perm(len(g.Nodes))

	// skip tree nodes, except roots, otherwise we'd end up with trees detached from the main graph
	for _, e := range g.Edges {
		if _, exists := treeRoots[e.From]; !exists {
			nodesToSkip[e.From] = struct{}{}
		}
		if _, exists := treeRoots[e.To]; !exists {
			nodesToSkip[e.To] = struct{}{}
		}
	}

	// tries to connect a random subset of 50% of the nodes
	nNodesToConnect := int(float64(len(indices)) * config.nodeSubsetPercentageToConnect)
	for i := 0; i < nNodesToConnect; i++ {
		n1 := g.Nodes[indices[i]]
		n2 := g.Nodes[indices[i+1]]
		if shouldConnectNodes(random, config, n1, n2, nodesToSkip, maxLength) {
			edge := g.Connect(n1, n2)
			edge.ID = layoutgraph.EntityID(len(g.Edges))
			if random.Float32() <= fuzzArrowheadProbability {
				edge.SourceArrowhead = layoutgraph.TriangleArrowhead
			}
			if random.Float32() <= fuzzArrowheadProbability {
				edge.TargetArrowhead = layoutgraph.TriangleArrowhead
			}
		}
	}
}

func shouldConnectNodes(random *rand.Rand, config graphFuzzerConfig, n1, n2 *layoutgraph.Node, nodesToSkip map[*layoutgraph.Node]struct{}, maxLength int) bool {
	if _, exists := nodesToSkip[n1]; exists {
		return false
	}
	if _, exists := nodesToSkip[n2]; exists {
		return false
	}
	if n1.DoesOverlapAt(n2, n1.TopLeft) || n2.DoesOverlapAt(n1, n2.TopLeft) {
		return false
	}
	d := geo.EuclideanDistance(n1.TopLeft.X, n1.TopLeft.Y, n2.TopLeft.X, n2.TopLeft.Y)
	p := d / float64(maxLength)
	p = 1 - p                                        // inverse probability so that longer edges have lower probability of happening, but still can happen
	p = math.Max(config.minConnectionProbability, p) // at least minConnectionProbability
	p = math.Min(config.maxConnectionProbability, p) // at most maxConnectionProbability
	return random.Float64() <= p
}
