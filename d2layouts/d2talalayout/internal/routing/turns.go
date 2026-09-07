package routing

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

type idealTurnAxis struct {
	isX bool
	val float64
}

// idealTurnAxes returns the midpoint axes where a route can turn between two
// node boxes without adding avoidable distance.
func idealTurnAxes(nodeA, nodeB *layoutgraph.Node) []idealTurnAxis {
	x1 := nodeA.TopLeft.X
	y1 := nodeA.TopLeft.Y
	x1b := nodeA.TopLeft.X + nodeA.Width
	y1b := nodeA.TopLeft.Y + nodeA.Height

	x2 := nodeB.TopLeft.X
	y2 := nodeB.TopLeft.Y
	x2b := nodeB.TopLeft.X + nodeB.Width
	y2b := nodeB.TopLeft.Y + nodeB.Height

	left := x2b < x1
	right := x1b < x2
	top := y2b < y1
	bottom := y1b < y2

	axes := make([]idealTurnAxis, 0)
	switch {
	case top && left:
		axes = append(axes,
			idealTurnAxis{isX: true, val: (x1 + x2b) / 2},
			idealTurnAxis{isX: false, val: (y1 + y2b) / 2},
		)
	case left && bottom:
		axes = append(axes,
			idealTurnAxis{isX: true, val: (x1 + x2b) / 2},
			idealTurnAxis{isX: false, val: (y2 + y1b) / 2},
		)
	case bottom && right:
		axes = append(axes,
			idealTurnAxis{isX: true, val: (x2 + x1b) / 2},
			idealTurnAxis{isX: false, val: (y2 + y1b) / 2},
		)
	case right && top:
		axes = append(axes,
			idealTurnAxis{isX: true, val: (x2 + x1b) / 2},
			idealTurnAxis{isX: false, val: (y1 + y2b) / 2},
		)
	case left:
		axes = append(axes, idealTurnAxis{isX: true, val: (x1 + x2b) / 2})
	case right:
		axes = append(axes, idealTurnAxis{isX: true, val: (x2 + x1b) / 2})
	case bottom:
		axes = append(axes, idealTurnAxis{isX: false, val: (y2 + y1b) / 2})
	case top:
		axes = append(axes, idealTurnAxis{isX: false, val: (y1 + y2b) / 2})
	}

	return axes
}
