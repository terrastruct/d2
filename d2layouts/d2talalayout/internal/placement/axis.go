package placement

import "github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

type layoutAxis uint8

const (
	invalidAxis layoutAxis = iota
	horizontalAxis
	verticalAxis
)

func (axis layoutAxis) valid() bool {
	return axis == horizontalAxis || axis == verticalAxis
}

func (axis layoutAxis) isHorizontal() bool {
	return axis == horizontalAxis
}

func (axis layoutAxis) opposite() layoutAxis {
	switch axis {
	case horizontalAxis:
		return verticalAxis
	case verticalAxis:
		return horizontalAxis
	default:
		return invalidAxis
	}
}

func axisForArrangement(arrangement layoutgraph.ClusterArrangement) layoutAxis {
	if arrangement == layoutgraph.Column {
		return horizontalAxis
	}
	return verticalAxis
}

type traversalDirection uint8

const (
	invalidDirection traversalDirection = iota
	forwardDirection
	backwardDirection
)

func (direction traversalDirection) valid() bool {
	return direction == forwardDirection || direction == backwardDirection
}

func (direction traversalDirection) isForward() bool {
	return direction == forwardDirection
}

func (direction traversalDirection) opposite() traversalDirection {
	switch direction {
	case forwardDirection:
		return backwardDirection
	case backwardDirection:
		return forwardDirection
	default:
		return invalidDirection
	}
}
