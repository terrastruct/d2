package placementcost

import (
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// AxisScore measures how strongly nodes share one visual axis.
func AxisScore(nodes layoutgraph.Nodes) float64 {
	if len(nodes) < 2 {
		return 1
	}
	if len(nodes) == 2 {
		if nodes[0].Orientation(nodes[1]).IsDiagonal() {
			return 0
		}
		return 1
	}

	largestWidth := math.Inf(-1)
	var largestWidthNode *layoutgraph.Node
	largestHeight := math.Inf(-1)
	var largestHeightNode *layoutgraph.Node
	for _, node := range nodes {
		if node.Width > largestWidth {
			largestWidth = node.Width
			largestWidthNode = node
		}
		if node.Height > largestHeight {
			largestHeight = node.Height
			largestHeightNode = node
		}
	}

	verticallyAligned := true
	for _, node := range nodes {
		if node == largestHeightNode {
			continue
		}
		if !node.Orientation(largestHeightNode).IsHorizontal() {
			verticallyAligned = false
			break
		}
	}
	if verticallyAligned {
		center := largestHeightNode.Center()
		score := 0.0
		for _, node := range nodes {
			if node == largestHeightNode {
				continue
			}
			nodeScore := 0.0
			for _, fraction := range []float64{0.25, 0.5, 0.75} {
				y := largestHeightNode.TopLeft.Y + fraction*largestHeightNode.Height
				if node.OverlapsLine(geo.NewPoint(center.X, y), geo.NewPoint(node.Center().X, y), 0) {
					nodeScore += 0.33
				}
			}
			if node.Height < 0.25*largestHeightNode.Height {
				nodeScore *= 3
			} else if node.Height < 0.75*largestHeightNode.Height {
				nodeScore *= 2
			}
			if nodeScore == 0.99 {
				score++
			} else {
				score += nodeScore
			}
		}
		return math.Max(0.33, score/float64(len(nodes)-1))
	}

	horizontallyAligned := true
	for _, node := range nodes {
		if node == largestWidthNode {
			continue
		}
		if !node.Orientation(largestWidthNode).IsVertical() {
			horizontallyAligned = false
			break
		}
	}
	if horizontallyAligned {
		center := largestWidthNode.Center()
		score := 0.0
		for _, node := range nodes {
			if node == largestWidthNode {
				continue
			}
			nodeScore := 0.0
			for _, fraction := range []float64{0.25, 0.5, 0.75} {
				x := largestWidthNode.TopLeft.X + fraction*largestWidthNode.Width
				if node.OverlapsLine(geo.NewPoint(x, center.Y), geo.NewPoint(x, node.Center().Y), 0) {
					nodeScore += 0.33
				}
			}
			if node.Width < 0.25*largestWidthNode.Width {
				nodeScore *= 3
			} else if node.Width < 0.75*largestWidthNode.Width {
				nodeScore *= 2
			}
			if nodeScore == 0.99 {
				score++
			} else {
				score += nodeScore
			}
		}
		return math.Max(0.33, score/float64(len(nodes)-1))
	}
	return 0
}
