package engine

import (
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func TestCheckOverlap(t *testing.T) {
	g := layoutgraph.NewGraph()

	n1 := g.AddNode(layoutgraph.NewNode(1, 100, 100))
	n1.TopLeft = geo.NewPoint(0, 0)
	if err := checkOverlap(g); err != nil {
		t.Fatal("Did not expect overlap for a single node")
	}

	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.TopLeft = geo.NewPoint(150, 150) // outside n1
	if err := checkOverlap(g); err != nil {
		t.Fatal("Nodes n1 and n2 do not overlap")
	}

	// full overlap, n3 hides n1
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	n3.TopLeft = geo.NewPoint(0, 0)
	if err := checkOverlap(g); err == nil {
		t.Fatal("Expected overlap of nodes n1 and n3")
	}

	// partial overlap and n3 extends outside n1
	n3.TopLeft = geo.NewPoint(50, 50)
	if err := checkOverlap(g); err == nil {
		t.Fatal("Expected overlap of nodes n1 and n3")
	}

	// n3 hides the bottom right of n1
	n3.Width = 50
	n3.Height = 50
	if err := checkOverlap(g); err == nil {
		t.Fatal("Expected overlap of nodes n1 and n3")
	}

	g.AddNewNodeToContainer(n1, n3)
	if err := checkOverlap(g); err != nil {
		t.Fatal("Did not expect overlap of child and container")
	}
}

func TestCheckContainers(t *testing.T) {
	start := layoutgraph.NewGraph()

	startC1 := start.AddNode(layoutgraph.NewNode(1, 100, 100))
	startC1.TopLeft = geo.NewPoint(0, 0)
	startC2 := start.AddNode(layoutgraph.NewNode(2, 100, 100))
	startC2.TopLeft = geo.NewPoint(500, 500)

	startN3 := start.AddNode(layoutgraph.NewNode(3, 30, 30))
	startN3.TopLeft = geo.NewPoint(10, 10)
	start.AddNewNodeToContainer(startC1, startN3)
	startN4 := start.AddNode(layoutgraph.NewNode(4, 30, 30))
	startN4.TopLeft = geo.NewPoint(60, 60)
	start.AddNewNodeToContainer(startC1, startN4)
	startN5 := start.AddNode(layoutgraph.NewNode(5, 30, 30))
	startN5.TopLeft = geo.NewPoint(535, 535)
	start.AddNewNodeToContainer(startC2, startN5)

	if err := checkContainers(start, start); err != nil {
		t.Fatal("Did not expect errors comparing to itself")
	}

	result := layoutgraph.NewGraph()
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check without nodes")
	}

	resultC1 := result.AddNode(layoutgraph.NewNode(1, 100, 100))
	resultC1.TopLeft = geo.NewPoint(0, 0)
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check with different container count")
	}
	resultC2 := result.AddNode(layoutgraph.NewNode(2, 100, 100))
	resultC2.TopLeft = geo.NewPoint(500, 500)
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check without container children")
	}

	resultN3 := result.AddNode(layoutgraph.NewNode(3, 30, 30))
	resultN3.TopLeft = geo.NewPoint(10, 10)
	result.AddNewNodeToContainer(resultC1, resultN3)
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check with wrong container children")
	}
	resultN4 := result.AddNode(layoutgraph.NewNode(4, 30, 30))
	resultN4.TopLeft = geo.NewPoint(60, 60)
	result.AddNewNodeToContainer(resultC1, resultN4)
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check wrong container children")
	}
	resultN5 := result.AddNode(layoutgraph.NewNode(5, 30, 30))
	resultN5.TopLeft = geo.NewPoint(535, 535)
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check if node is not descendant")
	}
	result.AddNewNodeToContainer(resultC2, resultN5)
	if err := checkContainers(start, result); err != nil {
		t.Fatal("Did not expect to fail container check if graphs are the same")
	}

	resultN5.TopLeft = geo.NewPoint(590, 535)
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check if container does not child")
	}

	resultN5.TopLeft = geo.NewPoint(535, 535)
	resultN5.ID = 6
	if err := checkContainers(start, result); err == nil {
		t.Fatal("Expected to fail container check if child changed")
	}
}

func TestCheckContainerSizes(t *testing.T) {
	g := layoutgraph.NewGraph()

	/* check if there is too much empty space in the container
	|                        |
	|  n1                    |
	|                        |
	|  n2                    |
	|                        |
	*/

	c1 := g.AddNode(layoutgraph.NewNode(1, 1000, 300))
	c1.TopLeft = geo.NewPoint(0, 0)

	n2 := g.AddNode(layoutgraph.NewNode(2, 100, 100))
	n2.TopLeft = geo.NewPoint(50, 50)
	n3 := g.AddNode(layoutgraph.NewNode(3, 100, 100))
	n3.TopLeft = geo.NewPoint(50, 150)

	g.AddNewNodeToContainer(c1, n2)
	g.AddNewNodeToContainer(c1, n3)

	c1.SetShape(shape.REAL_SQUARE_TYPE)
	if err := checkContainersSizes(g); err != nil {
		t.Fatal("Did not expect to fail for squared type")
	}

	c1.SetShape(shape.SQUARE_TYPE)
	if err := checkContainersSizes(g); err == nil {
		t.Fatal("Expected to fail container sizes check")
	}

	c1.Width = 200
	if err := checkContainersSizes(g); err != nil {
		t.Fatal("Expected to fail container sizes check")
	}
}
