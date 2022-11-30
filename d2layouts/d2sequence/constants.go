package d2sequence

// leaves at least 25 units of space on the left/right when computing the space required between actors
const HORIZONTAL_PAD = 50.

// leaves at least 25 units of space on the top/bottom when computing the space required between edges
const VERTICAL_PAD = 50.

const MIN_ACTOR_DISTANCE = 200.

// min vertical distance between edges
const MIN_EDGE_DISTANCE = 100.

// default size
const SPAN_WIDTH = 20.

// small pad so that edges don't touch lifelines and spans
const SPAN_EDGE_PAD = 5.

// as the spans start getting nested, their size grows
const SPAN_DEPTH_GROW_FACTOR = 10.

// when a span has a single edge
const MIN_SPAN_HEIGHT = MIN_EDGE_DISTANCE / 2.
