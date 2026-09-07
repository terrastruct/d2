package placementcost

import (
	"context"
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

const (
	scoringCancellationCheckInterval = 64
	sizelessDirectionDeltaFactor     = 0.25
	symmetryToleranceBand            = 1.0
)

const (
	// SideEdgeSpacing is the minimum spacing used to distinguish aligned side ports.
	SideEdgeSpacing = 40.0
	// IdealGapSize is the target separation between connected node boundaries.
	IdealGapSize = 2.5 * layoutgraph.ConnectedNodeGap
)

func scoringCancellationError(ctx context.Context, iteration int) error {
	if iteration%scoringCancellationCheckInterval != 0 {
		return nil
	}
	return checkScoringCancellation(ctx)
}

// Keep the interval check small enough to inline into the scoring loops. The
// context call and error formatting are only needed at a polling boundary.
func checkScoringCancellation(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("EdgeLength: %w", err)
	}
	return nil
}

type compassReading int

func directionCompass(direction geo.Orientation) compassReading {
	switch direction {
	case geo.BottomLeft:
		return -3
	case geo.Left:
		return -2
	case geo.TopLeft:
		return -1
	case geo.Top:
		return 0
	case geo.TopRight:
		return 1
	case geo.Right:
		return 2
	case geo.BottomRight:
		return 3
	case geo.Bottom:
		return 4
	default:
		return 0
	}
}

func compassDelta(first, second compassReading) compassReading {
	delta := second - first
	if delta > 4 {
		delta -= 8
	} else if delta < -4 {
		delta += 8
	}
	return delta
}

func compassAxisDelta(first, second compassReading) compassReading {
	first = (first + 4) % 4
	second = (second + 4) % 4
	delta := second - first
	if delta == 3 {
		return -1
	}
	return delta
}

func distanceToPoint(node *layoutgraph.Node, point *geo.Point, includeSizes bool) float64 {
	box := geo.Box{TopLeft: node.TopLeft}
	if includeSizes {
		box.Width, box.Height = node.Width, node.Height
	}
	pointBox := geo.Box{TopLeft: point}
	return distanceBetweenBoxes(box, pointBox)
}

func placementDistance(first, second *layoutgraph.Node, includeSizes bool) float64 {
	distance := first.DistanceTo(second, includeSizes)
	xCenter := math.Abs(first.TopLeft.X - second.TopLeft.X)
	yCenter := math.Abs(first.TopLeft.Y - second.TopLeft.Y)
	if includeSizes {
		xCenter = math.Abs((first.TopLeft.X+first.Width/2)-(second.TopLeft.X+second.Width/2)) / (first.Width + second.Width)
		yCenter = math.Abs((first.TopLeft.Y+first.Height/2)-(second.TopLeft.Y+second.Height/2)) / (first.Height + second.Height)
	}
	return distance + math.Min(xCenter, yCenter)/20
}

func distanceBetweenBoxes(first, second geo.Box) float64 {
	dx := intervalGap(first.TopLeft.X, first.TopLeft.X+first.Width, second.TopLeft.X, second.TopLeft.X+second.Width)
	dy := intervalGap(first.TopLeft.Y, first.TopLeft.Y+first.Height, second.TopLeft.Y, second.TopLeft.Y+second.Height)
	return geo.EuclideanDistance(0, 0, dx, dy)
}

func intervalGap(firstStart, firstEnd, secondStart, secondEnd float64) float64 {
	if firstEnd < secondStart {
		return secondStart - firstEnd
	}
	if secondEnd < firstStart {
		return firstStart - secondEnd
	}
	return 0
}

func sizelessOrientation(node, other *layoutgraph.Node) geo.Orientation {
	if node.TopLeft == nil || other.TopLeft == nil {
		return geo.NONE
	}
	if node.TopLeft.Y < other.TopLeft.Y {
		if node.TopLeft.X < other.TopLeft.X {
			return geo.TopLeft
		}
		if other.TopLeft.X < node.TopLeft.X {
			return geo.TopRight
		}
		return geo.Top
	}
	if other.TopLeft.Y < node.TopLeft.Y {
		if node.TopLeft.X < other.TopLeft.X {
			return geo.BottomLeft
		}
		if other.TopLeft.X < node.TopLeft.X {
			return geo.BottomRight
		}
		return geo.Bottom
	}
	if other.TopLeft.X < node.TopLeft.X {
		return geo.Right
	}
	if node.TopLeft.X < other.TopLeft.X {
		return geo.Left
	}
	return geo.NONE
}

func depth(node *layoutgraph.Node) int {
	if node == nil {
		return 0
	}
	return 1 + depth(node.Container)
}

func distanceBetweenTableColumns(graph *layoutgraph.Graph, edge *layoutgraph.Edge, from, to *layoutgraph.Node) float64 {
	fromPort, toPort, hasFromPort, hasToPort, orientation := edge.FacingTablePortValues(from, to)
	if orientation == geo.NONE {
		if from == nil {
			from = edge.From
		}
		if to == nil {
			to = edge.To
		}
		topLeft, bottomRight := layoutgraph.Nodes{from, to}.BoundingBox()
		return 2 * geo.EuclideanDistance(topLeft.X, topLeft.Y, bottomRight.X, bottomRight.Y)
	}
	multiplier := 2.0
	if orientation == geo.Left || orientation == geo.Right {
		multiplier = 1
	}
	if !hasFromPort {
		fromPort = geo.Point{
			X: from.TopLeft.X + from.Width/2,
			Y: from.TopLeft.Y + from.Height/2,
		}
	}
	if !hasToPort {
		toPort = geo.Point{
			X: to.TopLeft.X + to.Width/2,
			Y: to.TopLeft.Y + to.Height/2,
		}
	}
	gapCost := 0.0
	if math.Abs(fromPort.X-toPort.X) < IdealGapSize {
		gapCost = 2 * graph.CellSize
	}
	return multiplier * (gapCost + geo.EuclideanDistance(fromPort.X, fromPort.Y, toPort.X, toPort.Y))
}
