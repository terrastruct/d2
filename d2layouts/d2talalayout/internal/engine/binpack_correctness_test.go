package engine

import (
	"context"
	"log/slog"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/shape"
)

func TestAutolayoutKeepsContainerRouteOnFinalShapeBorder(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 400, 300)
	container.SetShape(shape.OVAL_TYPE)
	container.SetContainer(true)
	graph.AddNewNodeToContainer(nil, container)
	var previous *layoutgraph.Node
	for index := 0; index < 3; index++ {
		node := layoutgraph.NewNode(layoutgraph.EntityID(2+index), 30+float64(index%2)*20, 30)
		graph.AddNewNodeToContainer(container, node)
		if previous != nil {
			edge := graph.Connect(previous, node)
			edge.TargetArrowhead = layoutgraph.TriangleArrowhead
		}
		previous = node
	}
	for index := 0; index < 5; index++ {
		graph.AddNewNodeToContainer(container, layoutgraph.NewNode(layoutgraph.EntityID(20+index), 20+float64(index%3)*20, 20))
	}
	external := layoutgraph.NewNode(100, 40, 40)
	graph.AddNewNodeToContainer(nil, external)
	edge := graph.Connect(container, external)

	if _, err := layoutWithSnapshots(context.Background(), graph, 1, false); err != nil {
		t.Fatal(err)
	}
	endpoint := edge.Points[0]
	expected := shape.TraceToShapeBorder(container.Shape, endpoint, container.Center())
	if drift := geo.EuclideanDistance(endpoint.X, endpoint.Y, expected.X, expected.Y); drift > 2 {
		t.Fatalf("final route endpoint %v is %.1f pixels off the container border at %v", endpoint, drift, expected)
	}
}

func TestBinPackKeepsChildrenInsideAsymmetricContainer(t *testing.T) {
	tests := []struct {
		name        string
		config      graphFuzzerConfig
		containerID layoutgraph.EntityID
	}{
		{
			name: "step",
			config: graphFuzzerConfig{
				maxNodes:                      40,
				connectionIterations:          3,
				compactionFactor:              0.88,
				nodeSubsetPercentageToConnect: 0.49,
				minConnectionProbability:      0.25,
				maxConnectionProbability:      0.30,
				seed:                          290002619799509563,
			},
			containerID: 1,
		},
		{
			name: "diamond",
			config: graphFuzzerConfig{
				maxNodes:                      13,
				connectionIterations:          5,
				compactionFactor:              0.14,
				nodeSubsetPercentageToConnect: 0.39,
				minConnectionProbability:      0.39,
				maxConnectionProbability:      0.78,
				seed:                          46,
			},
			containerID: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := log.Leveled(withTestLogger(context.Background(), t), slog.LevelError)
			graph := createRandomGraph(ctx, tt.config)
			if _, err := layoutWithSnapshots(ctx, graph, tt.config.seed, false); err != nil {
				t.Fatal(err)
			}
			if err := checkContainersSizes(graph); err != nil {
				t.Fatal(err)
			}

			container := nodeByID(graph, tt.containerID)
			innerBox := container.InnerBox()
			for _, child := range graph.Containers[container] {
				if !layoutgraph.Covers(innerBox, &child.Box) {
					t.Fatalf("child %d escapes container %d's inner box", child.ID, container.ID)
				}
			}
		})
	}
}
